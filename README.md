# lab

Scripts, images and notes for disposable Linux machines: dev VMs, test
environments, and reproductions of issues in projects I work on.

My workstation is a Mac, so anything that needs Linux, KVM, or more CPU than a
laptop has runs on cloud VMs. This repository keeps the recipes for those
machines so that a fresh one is one command away.

## Conventions

- Nothing account-specific is tracked: no project IDs, project numbers,
  zones, emails or hostnames. Scripts read them from the environment or from a
  gitignored `.env` file.
- No long-lived credentials. Scripts use the operator's own `gcloud` login.
- Every commit passes `make lint`, which runs the pre-commit hooks:
  gitleaks, shellcheck, yamllint and markdownlint. CI runs the same target.

## Setup

```sh
make install-hooks
```

This installs the pre-commit hooks into your clone. Hooks are opt-in per
clone; CI enforces the same checks on every push and pull request.
