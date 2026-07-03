#!/usr/bin/env bash
set -euo pipefail

SUDO="sudo"
if [[ "${EUID}" -eq 0 ]]; then
  SUDO=""
fi

run_systemctl() {
  if command -v systemctl >/dev/null 2>&1; then
    # shellcheck disable=SC2086
    ${SUDO} systemctl "$@" >/dev/null 2>&1 || true
  fi
}

remove_path() {
  local path="$1"
  if [[ -e "${path}" || -L "${path}" ]]; then
    echo "Removing ${path}"
    # shellcheck disable=SC2086
    ${SUDO} rm -rf "${path}"
  fi
}

remove_managed_unit() {
  local path="$1"
  if [[ -f "${path}" ]] && grep -q "timesync-cli" "${path}"; then
    remove_path "${path}"
  fi
}

echo "Removing timesync-managed NTP/PTP configuration..."
if command -v timesync >/dev/null 2>&1; then
  # shellcheck disable=SC2086
  if ${SUDO} timesync uninstall --yes; then
    exit 0
  fi
fi

run_systemctl stop phc2sys.service
run_systemctl stop ptp4l.service
run_systemctl disable phc2sys.service
run_systemctl disable ptp4l.service

remove_path "/etc/systemd/system/chrony.service.d/timesync-cli.conf"
remove_path "/etc/systemd/system/chronyd.service.d/timesync-cli.conf"
remove_path "/etc/systemd/system/ptp4l.service.d/timesync-cli.conf"
remove_path "/etc/systemd/system/phc2sys.service.d/timesync-cli.conf"
remove_managed_unit "/etc/systemd/system/ptp4l.service"
remove_managed_unit "/etc/systemd/system/phc2sys.service"

for dir in \
  /etc/systemd/system/chrony.service.d \
  /etc/systemd/system/chronyd.service.d \
  /etc/systemd/system/ptp4l.service.d \
  /etc/systemd/system/phc2sys.service.d; do
  if [[ -d "${dir}" ]]; then
    # shellcheck disable=SC2086
    ${SUDO} rmdir "${dir}" >/dev/null 2>&1 || true
  fi
done

remove_path "/etc/timesync-cli"
run_systemctl daemon-reload
run_systemctl reset-failed chrony.service chronyd.service ptp4l.service phc2sys.service
run_systemctl try-restart chrony.service
run_systemctl try-restart chronyd.service

echo "Timesync-managed NTP/PTP configuration removed."
