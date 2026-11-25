# bootc dev VM

A persistent GCE VM that runs [bootc](https://github.com/bootc-dev/bootc)'s
booted (tmt) integration tests. It is set up the way bootc's CI sets up a
GitHub `ubuntu-26.04` runner. Keep it stopped when idle: its disk keeps the
container and cargo caches, which cut a cold `just build-fetch` (about 20
minutes) to almost nothing on the next run.

## Usage

Set `GCP_PROJECT` and `GCP_ZONE` in the environment or in `.env` (copy
[`.env.example`](../.env.example)). Then:

```sh
make bootc-vm-create     # create if missing, start, provision (about 10 minutes)
make bootc-vm-ssh        # shell on the VM
make bootc-vm-stop       # stop; disk and caches stay
make bootc-vm-start      # start again later
make bootc-vm-provision  # re-run the guest setup, e.g. after bootc-dev/actions changes
make bootc-vm-status
make bootc-vm-destroy    # delete the VM and its disk, after confirmation
```

`make bootc-vm-create` is idempotent: on an existing VM it starts it if needed
and re-runs the guest setup.

To run a command without an interactive shell, call the script directly:

```sh
bootc-vm/vm.sh ssh --command='cd bootc && just test-tmt readonly'
```

### Testing a bootc pull request

On the VM:

```sh
git clone https://github.com/bootc-dev/bootc && cd bootc
git fetch origin pull/NUMBER/head && git checkout FETCH_HEAD
just build-fetch
just build
just test-tmt readonly
```

## What the VM is

- Ubuntu 26.04 (`ubuntu-2604-lts-amd64`), `n2-standard-16`, nested
  virtualization, 200 GB `pd-balanced`. Optionally Spot
  (`BOOTC_VM_SPOT=true`): preemption stops the VM and keeps the disk.
- The guest setup ([`guest/setup.sh`](guest/setup.sh)) runs bootc-dev/actions'
  `bootc-host-setup` scripts with libvirt enabled, skipping the
  runner-only disk cleanup. It then installs what GitHub runners preinstall:
  podman, skopeo, jq, build headers, rustup (minimal profile) and tmt, pinned
  to the version in bootc's `.github/actions/install-tmt`, in a venv.
- The CI environment those scripts export (`LIBVIRT_DEFAULT_URI`, `ARCH`)
  lands in `~/.config/bootc-host.env`, which `~/.bashrc` loads.

`bootc-host-setup` runs with sudo from the `main` branch of
bootc-dev/actions, as in bootc's CI. Set `BOOTC_ACTIONS_REF` to a commit to
pin it.

## Access

- SSH goes through [IAP TCP forwarding](https://cloud.google.com/iap/docs/using-tcp-forwarding)
  with OS Login. Your key goes into your OS Login profile with a 24-hour
  expiry, never into project `ssh-keys` metadata. Sudo needs
  `roles/compute.osAdminLogin`, which project owners have.
- Two firewall rules apply only to VMs tagged `bootc-dev-vm`: one denies
  port 22 from anywhere, one allows it from IAP's range. The VM keeps an
  ephemeral external IP for outbound traffic only. `make bootc-vm-destroy`
  offers to delete the rules.
- The VM has no service account, so it holds no Google credentials.

## Idle poweroff

A systemd timer on the VM powers it off after 60 minutes without an SSH
session and with a 5-minute load average below 1. A guest poweroff is a GCE
stop, so only the disk is billed afterwards. A detached run (tmux, nohup)
counts as activity only while it keeps a CPU busy. Set
`BOOTC_VM_IDLE_MINUTES` (0 disables) and re-run `make bootc-vm-provision` to
change it.

## Costs

Approximate us-central1 list prices:

| State   | Cost                                                         |
|---------|--------------------------------------------------------------|
| Running | about $0.80/hour on demand (VM, disk and external IP)        |
| Stopped | about $20/month for the 200 GB disk                          |
| Spot    | the VM part typically costs 60 to 91% less; preemptible      |

A VM left running all day costs about $19, which is why idle poweroff exists.
