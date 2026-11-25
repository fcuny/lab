#!/usr/bin/env bash
# Powers the VM off once it has been idle for IDLE_MINUTES, so a forgotten VM
# stops costing compute (GCE treats a guest poweroff as a stop). Activity is
# an open SSH session or a 5-minute load average of 1 or more, so a detached
# run that keeps a CPU busy also counts. idle-poweroff.timer runs this.
set -euo pipefail

idle_minutes=${IDLE_MINUTES:-60}
stamp=/run/idle-poweroff/last-active

[ "$idle_minutes" -gt 0 ] || exit 0

active() {
  # One sshd(-session) process per session, titled "user@pts/N" or "user@notty".
  pgrep -f '^sshd(-session)?: [^ ]+@' >/dev/null && return 0
  awk '{ exit !($2 >= 1) }' /proc/loadavg
}

now=$(date +%s)
mkdir -p "${stamp%/*}"

# /run is a tmpfs, so the stamp starts fresh on every boot.
if active || [ ! -f "$stamp" ]; then
  echo "$now" >"$stamp"
  exit 0
fi

idle=$((now - $(cat "$stamp")))
if [ "$idle" -ge $((idle_minutes * 60)) ]; then
  echo "Idle for $((idle / 60)) minutes; powering off."
  systemctl poweroff
fi
