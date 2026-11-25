// Package bootcvm manages a persistent GCE VM that runs bootc's booted (tmt)
// tests. See README.md in this directory.
package bootcvm

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// Config comes from the environment (or .env); see .env.example.
type Config struct {
	Project     string
	Zone        string
	Name        string
	MachineType string
	DiskGB      int64
	Spot        bool
	Network     string
	IdleMinutes int
	ActionsRef  string
}

var validRef = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

// ConfigFromEnv builds a Config from getenv, applying defaults.
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	get := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}
	c := Config{
		Project:     getenv("GCP_PROJECT"),
		Zone:        getenv("GCP_ZONE"),
		Name:        get("BOOTC_VM_NAME", "bootc-dev"),
		MachineType: get("BOOTC_VM_MACHINE_TYPE", "n2-standard-16"),
		Network:     get("BOOTC_VM_NETWORK", "default"),
		ActionsRef:  get("BOOTC_ACTIONS_REF", "main"),
	}
	var errs []error
	if c.Project == "" {
		errs = append(errs, errors.New("set GCP_PROJECT in the environment or in .env"))
	}
	if c.Zone == "" {
		errs = append(errs, errors.New("set GCP_ZONE in the environment or in .env"))
	}
	var err error
	if c.DiskGB, err = strconv.ParseInt(get("BOOTC_VM_DISK_GB", "200"), 10, 64); err != nil || c.DiskGB < 10 {
		errs = append(errs, errors.New("BOOTC_VM_DISK_GB must be a whole number of at least 10"))
	}
	if c.Spot, err = strconv.ParseBool(get("BOOTC_VM_SPOT", "false")); err != nil {
		errs = append(errs, errors.New("BOOTC_VM_SPOT must be true or false"))
	}
	if c.IdleMinutes, err = strconv.Atoi(get("BOOTC_VM_IDLE_MINUTES", "60")); err != nil || c.IdleMinutes < 0 {
		errs = append(errs, errors.New("BOOTC_VM_IDLE_MINUTES must be a whole number of minutes (0 disables)"))
	}
	// The ref ends up in a remote shell command.
	if !validRef.MatchString(c.ActionsRef) {
		errs = append(errs, fmt.Errorf("BOOTC_ACTIONS_REF %q is not a branch, tag or commit", c.ActionsRef))
	}
	return c, errors.Join(errs...)
}
