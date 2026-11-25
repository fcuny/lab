#!/usr/bin/env bash
# Runs on the VM as the operator's user (bootc-vm/vm.sh provision copies it
# there). Sets the host up the way bootc's CI sets up a GitHub ubuntu-26.04
# runner, plus the tools those runners preinstall. Safe to re-run.
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

# Keep in sync with bootc's .github/actions/install-tmt.
tmt_version=1.80.0
actions_ref=${BOOTC_ACTIONS_REF:-main}
idle_minutes=${BOOTC_VM_IDLE_MINUTES:-60}

actions_dir=$HOME/.local/share/bootc-dev-actions
host_env=$HOME/.config/bootc-host.env
tmt_venv=$HOME/.local/share/tmt-venv

export DEBIAN_FRONTEND=noninteractive

step() { printf '\n==> %s\n' "$*"; }

# GitHub runners preinstall these; bootc-host-setup does not install them.
install_packages() {
  step "Installing packages"
  sudo apt-get update
  sudo apt-get install -y git curl tmux podman skopeo jq gcc pkg-config \
    libvirt-dev python3-dev python3-venv
}

fetch_actions() {
  step "Fetching bootc-dev/actions at $actions_ref"
  if [ ! -d "$actions_dir/.git" ]; then
    git init --quiet "$actions_dir"
    git -C "$actions_dir" remote add origin https://github.com/bootc-dev/actions
  fi
  git -C "$actions_dir" fetch --quiet --depth 1 origin "$actions_ref"
  git -C "$actions_dir" checkout --quiet --force --detach FETCH_HEAD
  git -C "$actions_dir" log -1 --format='%h %s'
}

# The same three steps as the bootc-host-setup action, with libvirt.
host_setup() {
  local ci=$actions_dir/bootc-host-setup/ci
  step "Running bootc-host-setup"
  mkdir -p "$(dirname "$host_env")"
  # The scripts append to $GITHUB_ENV; start empty so re-runs don't duplicate.
  : >"$host_env"
  export GITHUB_ENV=$host_env BOOTC_HOST_LIBVIRT=true
  (
    # shellcheck source=/dev/null
    . "$ci/install-deps.sh"
    # It prunes Docker images and removes SDKs that only exist on GitHub
    # runners; there is no Docker here.
    # shellcheck disable=SC2329
    free_ubuntu_disk() { :; }
    main
  )
  bash "$ci/workarounds.sh"
  bash "$ci/test-environment.sh"
}

install_rust() {
  step "Installing Rust"
  if [ ! -x "$HOME/.cargo/bin/rustup" ]; then
    curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs |
      sh -s -- -y --profile minimal --no-modify-path
  fi
  "$HOME/.cargo/bin/rustup" --version
}

install_tmt() {
  step "Installing tmt $tmt_version"
  [ -x "$tmt_venv/bin/python" ] || python3 -m venv "$tmt_venv"
  "$tmt_venv/bin/pip" install --quiet --upgrade pip
  "$tmt_venv/bin/pip" install --quiet "tmt[provision-virtual]==$tmt_version"
  mkdir -p "$HOME/.local/bin"
  ln -sfn "$tmt_venv/bin/tmt" "$HOME/.local/bin/tmt"
  "$HOME/.local/bin/tmt" --version
}

# Put cargo, ~/.local/bin and the CI environment (LIBVIRT_DEFAULT_URI, ...)
# in every shell. The block goes first in ~/.bashrc, before Ubuntu's
# interactive-only guard, so non-interactive ssh commands get it too.
configure_shell() {
  local rc=$HOME/.bashrc tmp
  local begin='# >>> lab bootc-vm >>>' end='# <<< lab bootc-vm <<<'
  step "Configuring $rc"
  touch "$rc"
  tmp=$(mktemp)
  {
    echo "$begin"
    cat <<'EOF'
[ -f "$HOME/.cargo/env" ] && . "$HOME/.cargo/env"
case ":$PATH:" in *":$HOME/.local/bin:"*) ;; *) PATH="$HOME/.local/bin:$PATH" ;; esac
if [ -f "$HOME/.config/bootc-host.env" ]; then
  set -a
  . "$HOME/.config/bootc-host.env"
  set +a
fi
EOF
    echo "$end"
    sed "/^$begin\$/,/^$end\$/d" "$rc"
  } >"$tmp"
  mv "$tmp" "$rc"
}

install_idle_poweroff() {
  step "Installing idle poweroff ($idle_minutes minutes; 0 disables)"
  sudo install -D -m 0755 "$here/idle-poweroff.sh" /usr/local/libexec/idle-poweroff
  sudo install -m 0644 "$here/idle-poweroff.service" "$here/idle-poweroff.timer" /etc/systemd/system/
  printf 'IDLE_MINUTES=%s\n' "$idle_minutes" | sudo tee /etc/default/idle-poweroff >/dev/null
  sudo systemctl daemon-reload
  sudo systemctl enable --now idle-poweroff.timer
}

install_packages
fetch_actions
host_setup
install_rust
install_tmt
configure_shell
install_idle_poweroff
step "Done. Host environment:"
cat "$host_env"
