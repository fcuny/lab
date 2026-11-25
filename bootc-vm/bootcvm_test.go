package bootcvm

import (
	"archive/tar"
	"bufio"
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

func env(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestConfigDefaults(t *testing.T) {
	c, err := ConfigFromEnv(env(map[string]string{"GCP_PROJECT": "p", "GCP_ZONE": "z"}))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		Project: "p", Zone: "z", Name: "bootc-dev", MachineType: "n2-standard-16",
		DiskGB: 200, Network: "default", IdleMinutes: 60, ActionsRef: "main",
	}
	if c != want {
		t.Errorf("ConfigFromEnv() = %+v, want %+v", c, want)
	}
}

func TestConfigOverrides(t *testing.T) {
	c, err := ConfigFromEnv(env(map[string]string{
		"GCP_PROJECT": "p", "GCP_ZONE": "z",
		"BOOTC_VM_NAME": "n", "BOOTC_VM_MACHINE_TYPE": "m", "BOOTC_VM_DISK_GB": "300",
		"BOOTC_VM_SPOT": "true", "BOOTC_VM_NETWORK": "net", "BOOTC_VM_IDLE_MINUTES": "0",
		"BOOTC_ACTIONS_REF": "0123abc",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		Project: "p", Zone: "z", Name: "n", MachineType: "m",
		DiskGB: 300, Spot: true, Network: "net", IdleMinutes: 0, ActionsRef: "0123abc",
	}
	if c != want {
		t.Errorf("ConfigFromEnv() = %+v, want %+v", c, want)
	}
}

func TestConfigErrors(t *testing.T) {
	base := map[string]string{"GCP_PROJECT": "p", "GCP_ZONE": "z"}
	tests := []struct {
		key, value, want string
	}{
		{"GCP_PROJECT", "", "GCP_PROJECT"},
		{"GCP_ZONE", "", "GCP_ZONE"},
		{"BOOTC_VM_DISK_GB", "big", "BOOTC_VM_DISK_GB"},
		{"BOOTC_VM_DISK_GB", "5", "BOOTC_VM_DISK_GB"},
		{"BOOTC_VM_SPOT", "maybe", "BOOTC_VM_SPOT"},
		{"BOOTC_VM_IDLE_MINUTES", "-1", "BOOTC_VM_IDLE_MINUTES"},
		{"BOOTC_VM_IDLE_MINUTES", "1h", "BOOTC_VM_IDLE_MINUTES"},
		{"BOOTC_ACTIONS_REF", "main; rm -rf ~", "BOOTC_ACTIONS_REF"},
		{"BOOTC_ACTIONS_REF", "$(id)", "BOOTC_ACTIONS_REF"},
	}
	for _, tt := range tests {
		vars := map[string]string{}
		for k, v := range base {
			vars[k] = v
		}
		vars[tt.key] = tt.value
		_, err := ConfigFromEnv(env(vars))
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s=%q: error = %v, want one mentioning %s", tt.key, tt.value, err, tt.want)
		}
	}
}

func testConfig() Config {
	return Config{
		Project: "p", Zone: "us-west1-b", Name: "vm", MachineType: "n2-standard-16",
		DiskGB: 200, Network: "default", IdleMinutes: 60, ActionsRef: "main",
	}
}

func TestInstance(t *testing.T) {
	inst := testConfig().Instance()
	if got, want := inst.GetMachineType(), "zones/us-west1-b/machineTypes/n2-standard-16"; got != want {
		t.Errorf("MachineType = %q, want %q", got, want)
	}
	if !inst.GetAdvancedMachineFeatures().GetEnableNestedVirtualization() {
		t.Error("nested virtualization is off")
	}
	disk := inst.GetDisks()[0]
	if !disk.GetBoot() || !disk.GetAutoDelete() || disk.GetInitializeParams().GetDiskSizeGb() != 200 {
		t.Errorf("boot disk = %v", disk)
	}
	if got := inst.GetDisks()[0].GetInitializeParams().GetDiskType(); got != "zones/us-west1-b/diskTypes/pd-balanced" {
		t.Errorf("DiskType = %q", got)
	}
	if !slices.Contains(inst.GetTags().GetItems(), Tag) {
		t.Errorf("tags %v lack %q", inst.GetTags().GetItems(), Tag)
	}
	var oslogin bool
	for _, item := range inst.GetMetadata().GetItems() {
		oslogin = oslogin || (item.GetKey() == "enable-oslogin" && item.GetValue() == "TRUE")
	}
	if !oslogin {
		t.Error("OS Login is not enabled")
	}
	if len(inst.GetServiceAccounts()) != 0 {
		t.Errorf("ServiceAccounts = %v, want none", inst.GetServiceAccounts())
	}
	if inst.GetScheduling() != nil {
		t.Errorf("on-demand VM has Scheduling %v", inst.GetScheduling())
	}
}

func TestInstanceSpot(t *testing.T) {
	c := testConfig()
	c.Spot = true
	s := c.Instance().GetScheduling()
	if s.GetProvisioningModel() != "SPOT" || s.GetInstanceTerminationAction() != "STOP" {
		t.Errorf("Scheduling = %v, want SPOT that stops on preemption", s)
	}
}

func TestFirewallRules(t *testing.T) {
	rules := testConfig().FirewallRules()
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(rules))
	}
	allow, deny := rules[0], rules[1]
	if allow.GetPriority() >= deny.GetPriority() {
		t.Errorf("allow priority %d must beat deny priority %d", allow.GetPriority(), deny.GetPriority())
	}
	if !slices.Equal(allow.GetSourceRanges(), []string{iapRange}) || len(allow.GetAllowed()) != 1 {
		t.Errorf("allow rule = %v", allow)
	}
	if !slices.Equal(deny.GetSourceRanges(), []string{"0.0.0.0/0"}) || len(deny.GetDenied()) != 1 {
		t.Errorf("deny rule = %v", deny)
	}
	for _, r := range rules {
		if !slices.Equal(r.GetTargetTags(), []string{Tag}) {
			t.Errorf("%s targets %v, want only %q", r.GetName(), r.GetTargetTags(), Tag)
		}
	}
}

func TestSetupCommand(t *testing.T) {
	want := "BOOTC_VM_IDLE_MINUTES=60 BOOTC_ACTIONS_REF=main bash ~/.local/share/lab-bootc-vm/setup.sh"
	if got := testConfig().setupCommand(); got != want {
		t.Errorf("setupCommand() = %q, want %q", got, want)
	}
}

func TestGuestArchive(t *testing.T) {
	var buf bytes.Buffer
	if err := writeGuestArchive(&buf); err != nil {
		t.Fatal(err)
	}
	modes := map[string]int64{}
	tr := tar.NewReader(&buf)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		modes[h.Name] = h.Mode
	}
	for _, name := range []string{"setup.sh", "idle-poweroff.sh", "idle-poweroff.service", "idle-poweroff.timer"} {
		if _, ok := modes[name]; !ok {
			t.Errorf("archive lacks %s; has %v", name, modes)
		}
	}
}

func TestPrompt(t *testing.T) {
	tests := []struct{ input, want string }{
		{"bootc-dev\n", "bootc-dev"},
		{"  y \n", "y"},
		{"no newline", "no newline"},
		{"", ""},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		got, err := prompt(bufio.NewReader(strings.NewReader(tt.input)), &out, "Q? ")
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want || out.String() != "Q? " {
			t.Errorf("prompt(%q) = %q (printed %q), want %q", tt.input, got, out.String(), tt.want)
		}
	}
}

func TestYes(t *testing.T) {
	for answer, want := range map[string]bool{"y": true, "Y": true, "yes": true, "": false, "n": false, "yep": false} {
		if got := yes(answer); got != want {
			t.Errorf("yes(%q) = %v, want %v", answer, got, want)
		}
	}
}
