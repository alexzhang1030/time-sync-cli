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

echo "Removing timesync config state..."
CONFIG_REMOVED=0
if command -v timesync >/dev/null 2>&1; then
  # shellcheck disable=SC2086
  if ${SUDO} timesync uninstall --yes >/dev/null 2>&1; then
    CONFIG_REMOVED=1
  fi
fi

if [[ "${CONFIG_REMOVED}" -eq 0 ]]; then
  echo "Stopping timesync-managed PTP services..."
  run_systemctl stop phc2sys.service
  run_systemctl stop ptp4l.service
  run_systemctl disable phc2sys.service
  run_systemctl disable ptp4l.service

  echo "Removing timesync-managed systemd files..."
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
fi

if command -v dpkg-query >/dev/null 2>&1 && dpkg-query -W -f='${Status}' timesync 2>/dev/null | grep -q "install ok installed"; then
  echo "Removing Debian package timesync..."
  # shellcheck disable=SC2086
  ${SUDO} apt purge -y timesync
elif command -v rpm >/dev/null 2>&1 && rpm -q timesync >/dev/null 2>&1; then
  echo "Removing RPM package timesync..."
  if command -v dnf >/dev/null 2>&1; then
    # shellcheck disable=SC2086
    ${SUDO} dnf remove -y timesync
  elif command -v yum >/dev/null 2>&1; then
    # shellcheck disable=SC2086
    ${SUDO} yum remove -y timesync
  else
    # shellcheck disable=SC2086
    ${SUDO} rpm -e timesync
  fi
elif [[ "$(command -v timesync 2>/dev/null || true)" == "/usr/local/bin/timesync" ]]; then
  echo "Removing /usr/local/bin/timesync..."
  remove_path "/usr/local/bin/timesync"
else
  echo "timesync package is already absent."
fi

run_systemctl daemon-reload

echo "Uninstall complete."
