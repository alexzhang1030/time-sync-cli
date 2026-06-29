# time-sync-cli

Linux CLI/TUI for managing NTP and PTP time synchronization on robots, industrial PCs, and embedded Linux deployments. It hides chrony/linuxptp complexity behind simple role-based configuration.

**Languages:** [English](README.md) · [简体中文](README.zh-CN.md)

## Implemented today

| Area | What works |
|------|------------|
| Detection | `timesync doctor` — OS, systemd, required binaries, interfaces, PTP hardware timestamping via `ethtool -T` |
| Status | `timesync status` — configured role, NTP/PTP offset and source, port state, path delay, systemd unit state |
| Configuration | `timesync apply auto\|master\|client` with `--dry-run`, optional `--ptp`, file backups |
| Interactive setup | `timesync tui` — arrow-key menu for doctor/status/apply; falls back to numbered prompts on non-TTY |
| RTC write-back | `rtcsync` in chrony configs; `phc2sys -w` in PTP drop-ins |
| Releases | Pre-built `linux/amd64` and `linux/arm64` binaries plus `.deb`/`.rpm` on [GitHub Releases](https://github.com/alexzhang1030/time-sync-cli/releases) |

## What are NTP and PTP?

### NTP (Network Time Protocol)

NTP synchronizes the **system clock** over IP networks. A device asks upstream NTP servers (or a local NTP server) for the current time, measures network delay, and gradually adjusts its system clock.

Typical accuracy: **milliseconds** (often 1–50 ms on a LAN, wider on the public internet).

Best for:

- General Linux hosts that need correct wall-clock time
- Devices with intermittent internet access
- Scenarios where millisecond-level accuracy is enough

`timesync` manages NTP through **chrony** (`chronyd` / `chronyc`).

### PTP (Precision Time Protocol, IEEE 1588)

PTP synchronizes time at the **network interface / PHC (PTP Hardware Clock)** layer using hardware timestamping on supported NICs. A grandmaster publishes time; slaves follow with much tighter bounds than NTP.

Typical accuracy: **sub-microsecond to tens of microseconds** (with hardware timestamping).

Best for:

- Robot fleets, motion control, industrial cameras, lidar/radar fusion
- Deterministic LAN topologies (single subnet or PTP-aware switches)
- Links where microsecond-level alignment matters

`timesync` manages PTP through **linuxptp** (`ptp4l`, `phc2sys`).

### NTP vs PTP (quick comparison)

| | NTP | PTP |
|---|---|---|
| Primary clock | System clock (CLOCK_REALTIME) | NIC PHC, then system clock |
| Typical accuracy | Milliseconds | Microseconds (with HW TS) |
| Upstream | NTP server / pool | PTP grandmaster |
| Backend | chrony | linuxptp |
| Requires special NIC | No | Yes (for hardware PTP) |

## Roles: master vs client (slave)

`timesync` uses three roles. **Master** and **client** are explicit; **auto** is a safe default for internet-connected devices.

| Role | NTP behavior | PTP behavior | When to use |
|------|--------------|--------------|-------------|
| `auto` | NTP client → internet pool | Optional PTP client if `--ptp` and HW supports it | Edge device with internet; never becomes master silently |
| `master` | NTP server for a CIDR | Optional PTP grandmaster with `--ptp` | Local time source for a cell / subnet |
| `client` | NTP client → `--source` | Optional PTP slave with `--ptp` | Follow a known upstream host |

### Enable auto mode (internet sync, safe default)

```bash
# Preview — NTP client to pool.ntp.org; optional PTP if --ptp and HW supports it
timesync apply auto --dry-run --iface eth0

# Apply
sudo timesync apply auto --iface eth0 --ntp-pool pool.ntp.org
sudo timesync apply auto --iface eth0 --ptp   # also enable PTP slave when HW supports it
```

`auto` never enables local NTP serving or PTP grandmaster — use `apply master` explicitly for that.

### Enable NTP master (serve time locally)

```bash
# Preview
timesync apply master --dry-run --iface eth0 --ntp-serve-cidr 192.168.0.0/24

# Apply (requires root)
sudo timesync apply master --iface eth0 --ntp-serve-cidr 192.168.0.0/24
```

This generates chrony config with `local stratum 8` and `allow <cidr>`, installs a systemd drop-in, and restarts `chronyd`.

### Enable PTP grandmaster (master + PTP)

```bash
sudo timesync apply master --iface eth0 --ptp
```

Verify hardware timestamping first:

```bash
timesync doctor   # check PTP capabilities per interface
```

### Enable NTP client (follow upstream)

```bash
sudo timesync apply client --iface eth0 --source 192.168.1.1
```

### Enable PTP slave (client + PTP)

```bash
sudo timesync apply client --iface eth0 --source 192.168.1.1 --ptp
```

PTP slaves follow the grandmaster via `ptp4l` unicast when `--source` is set (see `[unicast_master_table]` in generated config).

### Interactive setup

```bash
timesync tui
```

On an interactive terminal, `timesync tui` opens a full-screen menu:

- **Doctor** — system detection (OS, binaries, interfaces, PTP capabilities)
- **Status** — sync health, role, offset, systemd unit state
- **Apply** — guided role/interface configuration with dry-run, apply, or cancel
- **Quit**

Navigate with **↑/↓** (or `j`/`k`), confirm with **Enter**, go back with **Esc**. When stdin is not a TTY (pipes, CI, automation), the same flows are available via numbered prompts.

## Hardware clock (RTC) sync

There are three related clocks on a typical Linux device:

1. **System clock** (`CLOCK_REALTIME`) — what users and most applications see
2. **NIC PHC** — PTP hardware clock on the network interface (PTP path only)
3. **RTC** — battery-backed hardware clock on the motherboard

### What `timesync` does today

| Path | Mechanism | Direction |
|------|-----------|-------------|
| NTP (chrony) | `rtcsync` in generated chrony config | System clock → RTC (periodic write-back) |
| PTP (linuxptp) | `phc2sys -s <iface> -w` | PHC → system clock; `-w` also writes system time to RTC when stepping |

So after a successful sync:

- **NTP roles:** chrony keeps the system clock aligned and pushes corrections to the RTC via `rtcsync`.
- **PTP roles:** `phc2sys` disciplines the system clock from the PHC; with `-w`, large steps propagate to the RTC.

### Verify RTC / sync state

```bash
timesync status           # NTP + PTP sync health, port state, offset, path delay
chronyc tracking          # NTP offset and reference
pmc -u -b 0 'GET TIME_STATUS_NP'   # raw PTP offset (linuxptp)
timedatectl status        # system clock + RTC sync flag
```

## How it works (implementation)

```
┌─────────────┐     ┌──────────┐     ┌─────────────────────────────┐
│   timesync  │────▶│  planner │────▶│ /etc/timesync-cli/*.conf    │
│  CLI / TUI  │     │ (dry-run)│     │ systemd *.service.d drop-ins│
└─────────────┘     └──────────┘     └─────────────────────────────┘
       │                                        │
       ▼                                        ▼
┌─────────────┐                        ┌────────────────┐
│   doctor    │                        │ chronyd        │  NTP
│   status    │                        │ ptp4l + phc2sys│  PTP
└─────────────┘                        └────────────────┘
```

1. **Detection (`doctor`)** — reads `/etc/os-release`, checks systemd, locates binaries, lists `/sys/class/net` interfaces, runs `ethtool -T` for PTP hardware timestamping.
2. **Planning (`apply --dry-run`)** — renders role-specific chrony/ptp4l/phc2sys configs and systemd drop-ins under `/etc/timesync-cli/`. Does not touch vendor configs directly.
3. **Apply (`apply` without `--dry-run`)** — backs up existing files, writes configs, saves `state.json`, runs `systemctl daemon-reload`, enables and restarts affected units.
4. **Status** — read-only: `systemctl is-active`, `chronyc -c tracking`, configured role from `state.json`.

### Generated layout

```
/etc/timesync-cli/
├── chrony.conf          # NTP client or server config
├── ptp4l.conf           # PTP grandmaster or slave (when --ptp)
├── phc2sys.conf         # phc2sys reference args (informational)
├── state.json           # last applied role
└── backups/             # timestamped backups before overwrite

/etc/systemd/system/
├── chronyd.service.d/timesync-cli.conf
├── ptp4l.service.d/timesync-cli.conf
└── phc2sys.service.d/timesync-cli.conf
```

## Requirements

- Linux with systemd
- `chrony` (chronyd/chronyc)
- `linuxptp` (ptp4l, phc2sys) for PTP roles
- `ethtool` for PTP hardware capability detection

## Install

### Pre-built binaries

Download from the [latest release](https://github.com/alexzhang1030/time-sync-cli/releases/latest):

| Platform | Artifact |
|----------|----------|
| Linux x86_64 (`linux/amd64`) | [`timesync-linux-amd64`](https://github.com/alexzhang1030/time-sync-cli/releases/latest/download/timesync-linux-amd64) |
| Linux ARM64 (`linux/arm64`) | [`timesync-linux-arm64`](https://github.com/alexzhang1030/time-sync-cli/releases/latest/download/timesync-linux-arm64) |

```bash
# Example: amd64
curl -fsSL -o timesync https://github.com/alexzhang1030/time-sync-cli/releases/latest/download/timesync-linux-amd64
chmod +x timesync
sudo mv timesync /usr/local/bin/
```

### Distro packages (`.deb`, `.rpm`)

Tagged releases include native packages for `linux/amd64` and `linux/arm64`:

| Format | Example artifact |
|--------|------------------|
| Debian/Ubuntu (`.deb`) | `timesync_<version>_amd64.deb` |
| RHEL/Fedora (`.rpm`) | `timesync-<version>-1.x86_64.rpm` |

```bash
# Debian/Ubuntu (amd64)
curl -fsSLO https://github.com/alexzhang1030/time-sync-cli/releases/latest/download/timesync_<version>_amd64.deb
sudo apt install ./timesync_<version>_amd64.deb

# RHEL/Fedora (amd64)
curl -fsSLO https://github.com/alexzhang1030/time-sync-cli/releases/latest/download/timesync-<version>-1.x86_64.rpm
sudo dnf install ./timesync-<version>-1.x86_64.rpm
```

Packages install `/usr/bin/timesync` and declare runtime dependencies on `chrony` and `ethtool` (recommends `linuxptp` for PTP roles).

### Build from source

```bash
go build -o timesync ./cmd/timesync
sudo mv timesync /usr/local/bin/
```

## Commands

```bash
timesync doctor                                          # detect OS, tools, interfaces, PTP caps
timesync status                                          # sync health, role, NTP/PTP offset, port state
timesync apply auto [--iface eth0] [--ntp-pool pool.ntp.org] [--ptp] [--dry-run]
timesync apply master --iface eth0 [--ptp] [--ntp-serve-cidr 192.168.0.0/24] [--dry-run]
timesync apply client --iface eth0 --source <host> [--ptp] [--dry-run]
timesync tui                                             # guided interactive setup
timesync rollback                                        # restore files from last apply backup
```

Apply without `--dry-run` requires root (`sudo`) and will:

- write configs under `/etc/timesync-cli/`
- backup any existing target files to `/etc/timesync-cli/backups/`
- install systemd drop-ins and restart affected services

## Safety model

- Config generated under `/etc/timesync-cli/` — vendor chrony/ptp4l files are not mutated directly.
- Systemd drop-ins install dedicated unit overrides.
- `--dry-run` previews all planned changes without root writes.
- Applying changes requires `sudo` and backs up existing files before overwrite.
- `auto` will not enable local serving; use `apply master` explicitly.
- PTP requires hardware timestamping — verify with `timesync doctor`.

## Roadmap / not yet implemented

| Feature | Status |
|---------|--------|
| CI matrix build artifacts (`linux/amd64`, `linux/arm64`) | Done — see [releases](https://github.com/alexzhang1030/time-sync-cli/releases) |
| Distro packaging (`.deb`, `.rpm`) | Done |
| PTP unicast client (`--source` → ptp4l unicast master) | Done |
| Auto-detect PTP HW before enabling `--ptp` in apply | Done |
| Interactive confirmation before overwriting non-timesync configs | Planned |
| `timesync rollback` to restore backups | Done |
| Cluster leader election (multi-master avoidance) | Out of scope (by design) |
| Rich TUI (arrow-key menus) | Done |
| Deep PTP status parsing (port state, offset) | Done |

## Development

```bash
go test ./...
go build -o timesync ./cmd/timesync
```

## Supported assumptions

- systemd init
- chrony for NTP client/server
- linuxptp for PTP grandmaster/slave
- Network interfaces exposed via `/sys/class/net`

## License

MIT
