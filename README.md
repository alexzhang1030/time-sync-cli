# time-sync-cli

Linux CLI for managing NTP and PTP time synchronization on robot, industrial PC, and embedded Linux deployments. Hides chrony/linuxptp complexity behind simple role-based configuration.

## Requirements

- Linux with systemd
- `chrony` (chronyd/chronyc)
- `linuxptp` (ptp4l, phc2sys) for PTP roles
- `ethtool` for PTP hardware capability detection

## Install

```bash
go build -o timesync ./cmd/timesync
sudo mv timesync /usr/local/bin/
```

## Commands

```bash
timesync doctor                                          # detect OS, tools, interfaces, PTP caps
timesync status                                          # sync health, role, source, offset
timesync apply auto [--iface eth0] [--ntp-pool pool.ntp.org] [--ptp] [--dry-run]
timesync apply master --iface eth0 [--ptp] [--ntp-serve-cidr 192.168.0.0/24] [--dry-run]
timesync apply client --iface eth0 --source <host> [--ptp] [--dry-run]
timesync tui                                             # (coming soon)
```

## Roles

| Role | Behavior |
|------|----------|
| `auto` | Internet NTP sync via chrony; optional PTP when hardware supports it. Never silently becomes a master. |
| `master` | Serve time locally via NTP and/or PTP grandmaster. Requires explicit invocation. |
| `client` | Follow upstream NTP server or PTP master. |

## Safety model

- Config generated under `/etc/timesync-cli/` — vendor chrony/ptp4l files are not mutated directly.
- Systemd drop-ins install dedicated unit overrides.
- `--dry-run` previews all planned changes without root writes.
- `auto` will not enable local serving; use `apply master` explicitly.
- PTP requires hardware timestamping — verify with `timesync doctor`.

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
