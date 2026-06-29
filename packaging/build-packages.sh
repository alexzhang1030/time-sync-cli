#!/usr/bin/env bash
# Build .deb and .rpm packages from a compiled timesync binary.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${1:?usage: build-packages.sh <version> <arch> <binary> [outdir]}"
ARCH="${2:?usage: build-packages.sh <version> <arch> <binary> [outdir]}"
BINARY="${3:?usage: build-packages.sh <version> <arch> <binary> [outdir]}"
OUTDIR="${4:-dist/packages}"

case "$ARCH" in
  amd64 | arm64) ;;
  *)
    echo "unsupported arch: $ARCH (expected amd64 or arm64)" >&2
    exit 1
    ;;
esac

if [[ ! -f "$BINARY" ]]; then
  echo "binary not found: $BINARY" >&2
  exit 1
fi

VERSION="${VERSION#v}"
mkdir -p "$OUTDIR"
cp "$BINARY" timesync
chmod 755 timesync

export VERSION ARCH
envsubst < packaging/nfpm.yaml.tpl > /tmp/timesync-nfpm.yaml

nfpm pkg -f /tmp/timesync-nfpm.yaml --packager deb --target "$OUTDIR"
nfpm pkg -f /tmp/timesync-nfpm.yaml --packager rpm --target "$OUTDIR"

rm -f timesync /tmp/timesync-nfpm.yaml
ls -1 "$OUTDIR"
