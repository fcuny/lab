package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunUnknownRecipe(t *testing.T) {
	t.Chdir(t.TempDir())
	err := run(context.Background(), []string{"nope"})
	if err == nil || !strings.Contains(err.Error(), `unknown recipe "nope"`) {
		t.Errorf("run(nope) = %v, want an unknown recipe error", err)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	t.Chdir(t.TempDir())
	err := run(context.Background(), []string{"bootc-vm", "nope"})
	if err == nil || !strings.Contains(err.Error(), `unknown command "nope"`) {
		t.Errorf("run(bootc-vm nope) = %v, want an unknown command error", err)
	}
}

func TestRunHelp(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"-h"}, {"bootc-vm"}} {
		if err := run(context.Background(), args); err != nil {
			t.Errorf("run(%v) = %v, want nil", args, err)
		}
	}
}
