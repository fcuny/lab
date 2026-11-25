// Command lab manages the disposable machines in this repository.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	bootcvm "fcuny.net/lab/bootc-vm"
	"fcuny.net/lab/internal/dotenv"
)

const usage = `usage: lab RECIPE COMMAND

Recipes:
  bootc-vm   persistent GCE VM for bootc's booted tests (see bootc-vm/README.md)

Settings come from the environment, then from .env in the current directory.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lab:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Print(usage)
		return nil
	}
	if err := dotenv.Load(".env"); err != nil {
		return err
	}
	switch args[0] {
	case "bootc-vm":
		if len(args) == 1 {
			fmt.Print(bootcvm.Usage)
			return nil
		}
		return bootcvm.Main(ctx, args[1:])
	default:
		return fmt.Errorf("unknown recipe %q\n%s", args[0], usage)
	}
}
