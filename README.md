# time-sync-cli

Linux CLI/TUI for managing NTP and PTP time synchronization on robots, industrial PCs, and embedded Linux. Hides chrony/linuxptp complexity behind simple role-based configuration.

**Languages:** [English](README.md) · [简体中文](README.zh-CN.md)

## What it does

| Command | Purpose |
|---------|---------|
| `timesync doctor` | Detect OS, systemd, binaries, interfaces, PTP hardware timestamping |
| `timesync status` | Role-aware health dashboard, plain text, or JSON |
| `timesync apply auto\|master\|client` | Apply NTP/PTP role configs with `--dry-run` and backups |
| `timesync repair-clock` | Recover system time and PHC from RTC after an epoch reset |
| `timesync publish-gm-time-properties` | Align PHC and publish valid GM UTC offset on a PTP master |
| `timesync guard-ptp` | Periodic guard: manages `phc2sys`, syncs RTC, recovers PTP masters |
| `timesync tui` | Interactive arrow-key setup |

Pre-built binaries and `.deb`/`.rpm` packages are on [GitHub Releases](https://github.com/alexzhang1030/time-sync-cli/releases).

## NTP vs PTP

| | NTP | PTP |
|---|---|---|
| Clock | System clock (`CLOCK_REALTIME`) | NIC PHC, then system clock |
| Typical accuracy | Milliseconds | Microseconds (with hardware timestamping) |
| Upstream | NTP server / pool | PTP grandmaster |
| Backend | chrony | linuxptp (`ptp4l`, `phc2sys`) |
| Special NIC | No | Yes |

## Roles

| Role | NTP | PTP | Use for |
|------|-----|-----|---------|
| `auto` | Client → internet pool | Optional monitor (`--ptp`) | Edge devices with internet; never becomes master |
| `master` | Server for a CIDR | Optional grandmaster (`--ptp`) | Local time source for a cell/subnet |
| `client` | Client → `--source` | Optional slave (`--ptp`) | Follow a known upstream |

### Examples

```bash
# Auto (safe default)
sudo timesync apply auto --iface eth0 --ntp-pool pool.ntp.org

# NTP master
sudo timesync apply master --iface eth0 \
  --ntp-pool cn.pool.ntp.org --ntp-serve-cidr 192.168.0.0/24

# PTP grandmaster
sudo timesync apply master --iface eth0 --ptp \
  --ntp-pool cn.pool.ntp.org --ntp-serve-cidr 192.168.0.0/24

# PTP slave
sudo timesync apply client --iface eth0 --source 192.168.1.1 --ptp

# Preview any apply without changing the system
timesync apply master --dry-run --iface eth0 --ptp
```

Verify PTP hardware support first:

```bash
timesync doctor
```

## Clock sources

A typical Linux device has three clocks:

1. **System clock** (`CLOCK_REALTIME`) — wall-clock time
2. **NIC PHC** — PTP hardware clock on the network interface
3. **RTC** — battery-backed motherboard clock

`timesync` coordinates them like this:

- **NTP roles:** chrony keeps the system clock aligned and writes back to RTC via `rtcsync`.
- **PTP master:** chrony provides trusted system time; `phc2sys` disciplines PHC from `CLOCK_REALTIME`; `ptp4l` serves the PHC to clients.
- **PTP client:** `ptp4l` follows the grandmaster on the PHC; `phc2sys` disciplines the system clock from the PHC.

On PTP masters, `timesync` publishes `currentUtcOffsetValid=1` only after `phc2sys` has aligned the PHC to `System + TAI–UTC`. This happens automatically via:

- `timesync repair-clock` (master)
- `timesync publish-gm-time-properties`
- periodic `timesync guard-ptp` whenever the valid bit is missing

## Status

```bash
timesync status                  # dashboard on a TTY
timesync status --output plain   # stable text for logs/pipes
timesync status --output json    # structured output
```

The report includes configured role, clock source flow, NTP/PTP health, systemd unit state, and PHC/RTC residuals.

## Recover from an epoch clock reset

When the system clock falls back to ~1970, bootstrap from RTC and let PTP converge:

```bash
sudo timesync repair-clock
# or with an explicit interface
sudo timesync repair-clock --iface eth0
```

On PTP masters this also waits for PHC alignment and publishes GM time properties.

## How it works

```
┌─────────────┐     ┌──────────┐     ┌─────────────────────────────┐
│   timesync  │────▶│  planner │────▶│ /etc/timesync-cli/*.conf    │
│  CLI / TUI  │     │ (dry-run)│     │ systemd units and drop-ins  │
└─────────────┘     └──────────┘     └─────────────────────────────┘
       │                                        │
       ▼                                        ▼
┌─────────────┐                        ┌────────────────┐
│   doctor    │                        │ chrony         │  NTP
│   status    │                        │ ptp4l + phc2sys│  PTP
└─────────────┘                        └────────────────┘
```

1. **`doctor`** — detects OS, systemd, binaries, interfaces, PTP hardware timestamping.
2. **`apply --dry-run`** — renders role-specific configs and systemd units.
3. **`apply`** — backs up existing files, writes configs, saves `state.json`, reloads systemd, restarts services.
4. **`status`** — reads systemd, `chronyc`, `pmc`, PHC/RTC, and `state.json`.
5. **`guard-ptp`** — runs every 5 s to keep `phc2sys` state aligned with health.

Generated files:

```
/etc/timesync-cli/
├── chrony.conf
├── ptp4l.conf
├── phc2sys.conf
├── state.json
└── backups/

/etc/systemd/system/
├── chrony.service.d/timesync-cli.conf
├── ptp4l.service
├── phc2sys.service
├── timesync-ptp-guard.service
└── timesync-ptp-guard.timer
```

## Commands

```bash
timesync doctor
timesync status [--output auto|fancy|plain|json]
timesync apply auto [--iface eth0] [--ntp-pool pool.ntp.org] [--ptp] [--dry-run] [--yes]
timesync apply master --iface eth0 [--ptp] [--ntp-serve-cidr CIDR] [--dry-run] [--yes]
timesync apply client --iface eth0 --source <host> [--ptp] [--dry-run] [--yes]
sudo timesync repair-clock [--iface eth0]
sudo timesync publish-gm-time-properties [--config PATH] [--timeout 30s]
sudo timesync guard-ptp
timesync tui
timesync rollback
timesync uninstall [--dry-run] [--yes]
```

Apply requires root and backs up existing files before overwriting. Overwriting a file not created by `timesync` requires confirmation on a TTY or `--yes` in CI.

## Install

### Pre-built binaries

```bash
curl -fsSL -o timesync https://github.com/alexzhang1030/time-sync-cli/releases/latest/download/timesync-linux-amd64
chmod +x timesync
sudo install -m 0755 timesync /usr/bin/timesync
```

Replace `amd64` with `arm64` for ARM64.

### Packages

```bash
# Debian/Ubuntu
curl -fsSL https://raw.githubusercontent.com/alexzhang1030/time-sync-cli/main/scripts/install-deb.sh | bash

# Uninstall
curl -fsSL https://raw.githubusercontent.com/alexzhang1030/time-sync-cli/main/scripts/uninstall.sh | bash
```

Tagged releases also provide `.deb` and `.rpm` artifacts.

### Build from source

```bash
go build -o timesync ./cmd/timesync
sudo install -m 0755 timesync /usr/bin/timesync
```

## Requirements

- Linux with systemd
- `chrony`
- `linuxptp` (`ptp4l`, `phc2sys`) for PTP roles
- `ethtool` for PTP hardware detection

## Development

```bash
go test ./...
go build -o timesync ./cmd/timesync
```

## License

MIT
