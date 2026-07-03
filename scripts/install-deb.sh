#!/usr/bin/env bash
set -euo pipefail

REPO="alexzhang1030/time-sync-cli"
VERSION="${1:-latest}"

case "$(dpkg --print-architecture)" in
  amd64)
    ARCH="amd64"
    ;;
  arm64 | aarch64)
    ARCH="arm64"
    ;;
  *)
    echo "unsupported Debian architecture: $(dpkg --print-architecture)" >&2
    exit 1
    ;;
esac

if [[ "${VERSION}" == "latest" ]]; then
  LATEST_URL="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest")"
  VERSION="$(basename "${LATEST_URL}")"
fi

VERSION="${VERSION#v}"
RELEASE_URL="https://github.com/${REPO}/releases/download/v${VERSION}"
FILE_NAME="timesync_${VERSION}_${ARCH}.deb"

TMP_DEB="/tmp/${FILE_NAME}"
trap 'rm -f "${TMP_DEB}"' EXIT

echo "Downloading ${FILE_NAME}..."
curl -fsSL -o "${TMP_DEB}" "${RELEASE_URL}/${FILE_NAME}"

echo "Installing ${TMP_DEB}..."
sudo apt install -y "${TMP_DEB}"

echo "Installed:"
timesync --version 2>/dev/null || command -v timesync
