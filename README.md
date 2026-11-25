# lab

Scripts, images and notes for disposable Linux machines: dev VMs, test
environments, and reproductions of issues in projects I work on.

My workstation is a Mac, so anything that needs Linux, KVM, or more CPU than a
laptop has runs on cloud VMs. This repository keeps the recipes for those
machines so that a fresh one is one command away.

## Recipes

- [`bootc-vm/`](bootc-vm/README.md): a persistent GCE VM for bootc's booted
  tmt tests, stopped when idle.

## Conventions

- Nothing account-specific is tracked: no project IDs, project numbers,
  zones, emails or hostnames. Scripts read them from the environment or from a
  gitignored `.env` file.
- No long-lived credentials. Scripts use the operator's own `gcloud` login.
- Languages, in order of preference: Go, Python, shell. Shell is fine for
  short glue. Once a script grows long or complex, write it in Go, or in
  Python with PEP 723 inline metadata run through `uv`. Go and Python code
  may use the Google Cloud SDK instead of calling `gcloud`. Scripts that run
  on a fresh VM before any toolchain exists stay in shell.
- Every commit passes `make lint`, which runs the pre-commit hooks:
  gitleaks, shellcheck, yamllint and markdownlint, and `make check`, which
  builds, vets, staticchecks and tests the Go code. CI runs both targets.

## Setup

```sh
make install-hooks
```

This installs the pre-commit hooks into your clone. Hooks are opt-in per
clone; CI enforces the same checks on every push and pull request.

## History

Earlier infrastructure lives in two archived, private repositories:

- `fcuny/infra`: Ansible for long-lived hosts (2018 to 2026).
- `fcuny/infrastructure`: Puppet (2019 to 2022).

This repository starts fresh and shares no history with them.
