package bootcvm

import (
	"archive/tar"
	"bufio"
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	"fcuny.net/lab/internal/gce"
)

//go:embed guest
var guestFiles embed.FS

// guestDir is where provision copies the guest scripts, relative to the
// operator's home on the VM.
const guestDir = ".local/share/lab-bootc-vm"

// Usage lists the commands.
const Usage = `usage: lab bootc-vm COMMAND

  create     create the VM if missing, start it, and provision it
  provision  (re)run the guest setup on a running VM
  start      start the VM and wait for SSH
  stop       stop the VM; the disk and its caches are kept
  ssh        open a shell on the VM; extra arguments go to gcloud compute ssh
  status     print the VM's status
  destroy    delete the VM and its disk, after confirmation
`

// Main runs one command.
func Main(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing command\n%s", Usage)
	}
	commands := map[string]func(*vm, context.Context, []string) error{
		"create":    (*vm).create,
		"provision": func(v *vm, ctx context.Context, _ []string) error { return v.provision(ctx) },
		"start":     (*vm).start,
		"stop":      (*vm).stop,
		"ssh":       (*vm).ssh,
		"status":    (*vm).status,
		"destroy":   (*vm).destroy,
	}
	run, ok := commands[args[0]]
	if !ok {
		return fmt.Errorf("unknown command %q\n%s", args[0], Usage)
	}
	cfg, err := ConfigFromEnv(os.Getenv)
	if err != nil {
		return err
	}
	client, err := gce.New(ctx, cfg.Project, cfg.Zone)
	if err != nil {
		return err
	}
	defer client.Close()
	v := &vm{cfg: cfg, gce: client, in: bufio.NewReader(os.Stdin), out: os.Stdout}
	return run(v, ctx, args[1:])
}

type vm struct {
	cfg Config
	gce *gce.Client
	in  *bufio.Reader
	out io.Writer
}

func (v *vm) printf(format string, args ...any) {
	fmt.Fprintf(v.out, format, args...)
}

func (v *vm) create(ctx context.Context, _ []string) error {
	status, err := v.gce.InstanceStatus(ctx, v.cfg.Name)
	if err != nil {
		return err
	}
	if err := v.ensureFirewall(ctx); err != nil {
		return err
	}
	switch status {
	case "":
		v.printf("Creating %s...\n", v.cfg.Name)
		if err := v.gce.CreateInstance(ctx, v.cfg.Instance()); err != nil {
			return err
		}
	case "RUNNING":
		v.printf("%s already exists and is running.\n", v.cfg.Name)
	default:
		v.printf("Starting %s (%s)...\n", v.cfg.Name, status)
		if err := v.gce.StartInstance(ctx, v.cfg.Name); err != nil {
			return err
		}
	}
	if err := v.waitForSSH(ctx); err != nil {
		return err
	}
	return v.provision(ctx)
}

func (v *vm) ensureFirewall(ctx context.Context) error {
	for _, rule := range v.cfg.FirewallRules() {
		exists, err := v.gce.FirewallExists(ctx, rule.GetName())
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		v.printf("Creating firewall rule %s...\n", rule.GetName())
		if err := v.gce.CreateFirewall(ctx, rule); err != nil {
			return err
		}
	}
	return nil
}

func (v *vm) waitForSSH(ctx context.Context) error {
	v.printf("Waiting for SSH on %s...\n", v.cfg.Name)
	const attempts = 30
	for i := 1; ; i++ {
		err := v.gce.SSH(ctx, v.cfg.Name, nil, io.Discard, io.Discard,
			"--command=true", "--", "-o", "ConnectTimeout=10")
		if err == nil {
			return nil
		}
		if i == attempts {
			return fmt.Errorf("SSH to %s did not come up after %d attempts: %w", v.cfg.Name, attempts, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}

// provision copies the guest scripts over one SSH session's stdin, then runs
// them in a second session so the setup never reads the archive as input.
func (v *vm) provision(ctx context.Context) error {
	pr, pw := io.Pipe()
	go func() { pw.CloseWithError(writeGuestArchive(pw)) }()
	copyCmd := fmt.Sprintf("rm -rf ~/%[1]s && mkdir -p ~/%[1]s && tar -xf - -C ~/%[1]s", guestDir)
	if err := v.gce.SSH(ctx, v.cfg.Name, pr, os.Stdout, os.Stderr, "--command="+copyCmd); err != nil {
		return fmt.Errorf("copy guest scripts: %w", err)
	}
	return v.gce.Interactive(ctx, v.cfg.Name, "--command="+v.cfg.setupCommand())
}

func (c Config) setupCommand() string {
	return fmt.Sprintf("BOOTC_VM_IDLE_MINUTES=%d BOOTC_ACTIONS_REF=%s bash ~/%s/setup.sh",
		c.IdleMinutes, c.ActionsRef, guestDir)
}

func writeGuestArchive(w io.Writer) error {
	guest, err := fs.Sub(guestFiles, "guest")
	if err != nil {
		return err
	}
	tw := tar.NewWriter(w)
	if err := tw.AddFS(guest); err != nil {
		return err
	}
	return tw.Close()
}

func (v *vm) start(ctx context.Context, _ []string) error {
	if err := v.gce.StartInstance(ctx, v.cfg.Name); err != nil {
		return err
	}
	return v.waitForSSH(ctx)
}

func (v *vm) stop(ctx context.Context, _ []string) error {
	return v.gce.StopInstance(ctx, v.cfg.Name)
}

func (v *vm) ssh(ctx context.Context, args []string) error {
	return v.gce.Interactive(ctx, v.cfg.Name, args...)
}

func (v *vm) status(ctx context.Context, _ []string) error {
	status, err := v.gce.InstanceStatus(ctx, v.cfg.Name)
	if err != nil {
		return err
	}
	if status == "" {
		status = "MISSING"
	}
	v.printf("%s\n", status)
	return nil
}

func (v *vm) destroy(ctx context.Context, _ []string) error {
	status, err := v.gce.InstanceStatus(ctx, v.cfg.Name)
	if err != nil {
		return err
	}
	if status == "" {
		v.printf("%s does not exist.\n", v.cfg.Name)
	} else {
		answer, err := prompt(v.in, v.out, fmt.Sprintf("Delete VM %s and its disk (all caches)? Type the VM name to confirm: ", v.cfg.Name))
		if err != nil {
			return err
		}
		if answer != v.cfg.Name {
			return errors.New("not deleting")
		}
		v.printf("Deleting %s...\n", v.cfg.Name)
		if err := v.gce.DeleteInstance(ctx, v.cfg.Name); err != nil {
			return err
		}
	}
	if err := v.offerFirewallCleanup(ctx); err != nil {
		return err
	}
	v.printf("Your SSH key is in your OS Login profile, not project metadata, and expires within 24 hours.\n")
	return nil
}

func (v *vm) offerFirewallCleanup(ctx context.Context) error {
	var existing []string
	for _, rule := range v.cfg.FirewallRules() {
		exists, err := v.gce.FirewallExists(ctx, rule.GetName())
		if err != nil {
			return err
		}
		if exists {
			existing = append(existing, rule.GetName())
		}
	}
	if len(existing) == 0 {
		return nil
	}
	others, err := v.gce.InstancesWithTag(ctx, Tag)
	if err != nil {
		return err
	}
	if len(others) > 0 {
		v.printf("Keeping the %s firewall rules: %s still carry the tag.\n", Tag, strings.Join(others, ", "))
		return nil
	}
	answer, err := prompt(v.in, v.out, fmt.Sprintf("Delete the firewall rules %s? [y/N] ", strings.Join(existing, " and ")))
	if err != nil {
		return err
	}
	if !yes(answer) {
		return nil
	}
	for _, name := range existing {
		v.printf("Deleting firewall rule %s...\n", name)
		if err := v.gce.DeleteFirewall(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

// prompt prints the question and returns the answer without surrounding
// whitespace. End of input counts as an empty answer.
func prompt(in *bufio.Reader, out io.Writer, question string) (string, error) {
	fmt.Fprint(out, question)
	line, err := in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func yes(answer string) bool {
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes")
}
