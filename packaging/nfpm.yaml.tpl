# nfpm package template — render with envsubst (VERSION, ARCH).
name: timesync
arch: ${ARCH}
platform: linux
version: ${VERSION}
version_schema: semver
section: utils
priority: optional
maintainer: "Alex <49969959+alexzhang1030@users.noreply.github.com>"
description: |
  Linux CLI for NTP/PTP time synchronization management on robots,
  industrial PCs, and embedded Linux deployments.
vendor: Alex Zhang
homepage: https://github.com/alexzhang1030/time-sync-cli
license: MIT

depends:
  - chrony
  - ethtool

recommends:
  - linuxptp

contents:
  - src: timesync
    dst: /usr/bin/timesync
    file_info:
      mode: 0755

deb:
  compression: gzip

rpm:
  compression: gzip
