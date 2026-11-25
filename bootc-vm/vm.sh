#!/usr/bin/env bash
# Manage a persistent GCE VM that runs bootc's booted (tmt) tests.
# See bootc-vm/README.md. Usage: bootc-vm/vm.sh COMMAND [ARGS...]
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo=$(cd "$here/.." && pwd)

# Google's IAP TCP forwarding range: https://cloud.google.com/iap/docs/using-tcp-forwarding
iap_range=35.235.240.0/20
tag=bootc-dev-vm
guest_dir='.local/share/lab-bootc-vm'

usage() {
  cat <<EOF
usage: $0 COMMAND

  create     create the VM if missing, start it, and provision it
  provision  (re)run the guest setup on a running VM
  start      start the VM and wait for SSH
  stop       stop the VM; the disk and its caches are kept
  ssh        open a shell on the VM; extra arguments go to gcloud compute ssh
  status     print the VM's status
  destroy    delete the VM and its disk, after confirmation
EOF
}

# Read KEY=VALUE lines from the repo's .env. Variables already set in the
# environment win over the file.
load_env() {
  local file="$repo/.env" key value
  [ -f "$file" ] || return 0
  while IFS='=' read -r key value || [ -n "$key" ]; do
    case "$key" in '' | '#'*) continue ;; esac
    if ! [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
      echo "$file: invalid variable name: $key" >&2
      exit 1
    fi
    value=${value%\"}
    value=${value#\"}
    [ -n "${!key+x}" ] || export "$key=$value"
  done <"$file"
}

load_config() {
  load_env
  : "${GCP_PROJECT:?set GCP_PROJECT in the environment or in .env}"
  : "${GCP_ZONE:?set GCP_ZONE in the environment or in .env}"
  name=${BOOTC_VM_NAME:-bootc-dev}
  machine_type=${BOOTC_VM_MACHINE_TYPE:-n2-standard-16}
  disk_gb=${BOOTC_VM_DISK_GB:-200}
  spot=${BOOTC_VM_SPOT:-false}
  network=${BOOTC_VM_NETWORK:-default}
  idle_minutes=${BOOTC_VM_IDLE_MINUTES:-60}
  actions_ref=${BOOTC_ACTIONS_REF:-main}
  if ! [[ "$idle_minutes" =~ ^[0-9]+$ ]]; then
    echo "BOOTC_VM_IDLE_MINUTES must be a whole number of minutes" >&2
    exit 1
  fi
  if ! [[ "$actions_ref" =~ ^[A-Za-z0-9._/-]+$ ]]; then
    echo "BOOTC_ACTIONS_REF must be a branch, tag or commit" >&2
    exit 1
  fi
}

gc() { gcloud --project="$GCP_PROJECT" "$@"; }

# OS Login and IAP: the key goes to the operator's OS Login profile (not
# project metadata) and expires on its own; port 22 is never exposed to the
# internet.
vm_ssh() {
  gc compute ssh "$name" --zone="$GCP_ZONE" --tunnel-through-iap \
    --ssh-key-expire-after=24h "$@"
}

# Prints the VM's status (RUNNING, TERMINATED, ...), or nothing if it does
# not exist. Fails on API or auth errors instead of reporting "missing".
vm_status() {
  gc compute instances list --zones="$GCP_ZONE" \
    --filter="name=($name)" --format='value(status)'
}

firewall_rule_exists() {
  [ -n "$(gc compute firewall-rules list --filter="name=($1)" --format='value(name)')" ]
}

# default-allow-ssh on the default network admits 0.0.0.0/0. These rules
# apply only to VMs with our tag: deny SSH from anywhere, allow it from IAP.
# Lower priority numbers win.
ensure_firewall() {
  if ! firewall_rule_exists "$tag-allow-iap-ssh"; then
    gc compute firewall-rules create "$tag-allow-iap-ssh" --network="$network" \
      --direction=INGRESS --action=ALLOW --rules=tcp:22 --priority=900 \
      --source-ranges="$iap_range" --target-tags="$tag" \
      --description="SSH to $tag VMs through IAP only"
  fi
  if ! firewall_rule_exists "$tag-deny-ssh"; then
    gc compute firewall-rules create "$tag-deny-ssh" --network="$network" \
      --direction=INGRESS --action=DENY --rules=tcp:22 --priority=1000 \
      --source-ranges=0.0.0.0/0 --target-tags="$tag" \
      --description="Block direct SSH to $tag VMs; see $tag-allow-iap-ssh"
  fi
}

wait_for_ssh() {
  local i
  echo "Waiting for SSH on $name..."
  for i in $(seq 1 30); do
    if vm_ssh --command=true -- -o ConnectTimeout=10 >/dev/null 2>&1; then
      return 0
    fi
    sleep 10
  done
  echo "SSH to $name did not come up after $i attempts" >&2
  return 1
}

cmd_create() {
  local status
  status=$(vm_status)
  ensure_firewall
  if [ -z "$status" ]; then
    local -a args=(
      --zone="$GCP_ZONE"
      --machine-type="$machine_type"
      --image-family=ubuntu-2604-lts-amd64
      --image-project=ubuntu-os-cloud
      --boot-disk-size="${disk_gb}GB"
      --boot-disk-type=pd-balanced
      --enable-nested-virtualization
      --network="$network"
      --tags="$tag"
      --metadata=enable-oslogin=TRUE
      # The VM never calls Google APIs, so it gets no credentials.
      --no-service-account
      --no-scopes
    )
    if [ "$spot" = true ]; then
      # A preempted Spot VM stops instead of being deleted, so the disk stays.
      args+=(--provisioning-model=SPOT --instance-termination-action=STOP)
    fi
    gc compute instances create "$name" "${args[@]}"
  elif [ "$status" != RUNNING ]; then
    gc compute instances start "$name" --zone="$GCP_ZONE"
  else
    echo "$name already exists and is running."
  fi
  wait_for_ssh
  cmd_provision
}

cmd_provision() {
  # Copy the guest scripts over the ssh session's stdin, then run them in a
  # second session so the setup never reads the tarball as input.
  tar -C "$here/guest" -cf - . |
    vm_ssh --command="rm -rf ~/$guest_dir && mkdir -p ~/$guest_dir && tar -xf - -C ~/$guest_dir"
  vm_ssh --command="BOOTC_VM_IDLE_MINUTES=$idle_minutes BOOTC_ACTIONS_REF=$actions_ref bash ~/$guest_dir/setup.sh"
}

cmd_start() {
  gc compute instances start "$name" --zone="$GCP_ZONE"
  wait_for_ssh
}

cmd_stop() {
  gc compute instances stop "$name" --zone="$GCP_ZONE"
}

cmd_ssh() {
  vm_ssh "$@"
}

cmd_status() {
  local status
  status=$(vm_status)
  echo "${status:-MISSING}"
}

confirm() {
  local answer
  read -r -p "$1 [y/N] " answer
  [ "$answer" = y ] || [ "$answer" = Y ]
}

cmd_destroy() {
  local answer
  if [ -n "$(vm_status)" ]; then
    read -r -p "Delete VM $name and its disk (all caches)? Type the VM name to confirm: " answer
    if [ "$answer" != "$name" ]; then
      echo "Not deleting." >&2
      exit 1
    fi
    gc compute instances delete "$name" --zone="$GCP_ZONE" --quiet
  else
    echo "$name does not exist."
  fi

  if firewall_rule_exists "$tag-allow-iap-ssh" || firewall_rule_exists "$tag-deny-ssh"; then
    if [ -n "$(gc compute instances list --filter="tags.items=$tag" --format='value(name)')" ]; then
      echo "Keeping the $tag firewall rules: other VMs still carry the tag."
    elif confirm "Delete the $tag-allow-iap-ssh and $tag-deny-ssh firewall rules?"; then
      local rule
      for rule in "$tag-allow-iap-ssh" "$tag-deny-ssh"; do
        if firewall_rule_exists "$rule"; then
          gc compute firewall-rules delete "$rule" --quiet
        fi
      done
    fi
  fi
  echo "Your SSH key is in your OS Login profile, not project metadata, and expires within 24 hours."
}

main() {
  local cmd=${1:-}
  case "$cmd" in
    create | provision | start | stop | ssh | status | destroy) ;;
    -h | --help | help | '')
      usage
      exit 0
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
  shift
  load_config
  "cmd_$cmd" "$@"
}

main "$@"
