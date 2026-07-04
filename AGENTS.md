# AGENTS.md

Repository guide for agents working on `time-sync-cli`.

## Operating Rules

- Use `rtk` as the prefix for shell commands in this workspace.
- Treat `/usr/bin/timesync` as the installed binary path for generated systemd units.
- Keep role changes explicit: `auto`, `master`, and `client` have different clock writers.
- Validate live machines with `sudo timesync status` after every apply.
- For PTP roles, verify `ptp4l`, `phc2sys`, `phc_ctl`, `pmc`, `ethtool`, and hardware timestamping before applying.
- Preserve user changes in the working tree. Add focused patches and tests.
- Run `go test ./...` and `git diff --check` before reporting a code change as verified.

## Fast Configuration Skill

Use the repository skill at `skills/timesync-quick-config/SKILL.md` for:

- configuring NTP/PTP master and PTP slave fleets,
- recovering a host from an epoch clock reset,
- validating status output after role changes,
- preparing a master/slave PTP fleet.

## Fleet Reference Topology

Use placeholders for user-specific hostnames, interfaces, and subnets in repository docs:

- `master-host`: NTP + PTP master on `<master-iface>`, PTP/NTP network `<ptp-cidr>`, master IP `<master-ip>`.
- `slave-a`: PTP slave on `<slave-a-iface>`, source `<master-ip>`.
- `slave-b`: PTP slave on `<slave-b-iface>`, source `<master-ip>`.

Apply commands:

```bash
sudo timesync apply master --iface <master-iface> --ptp --ntp-pool cn.pool.ntp.org --ntp-serve-cidr <ptp-cidr> --yes
sudo timesync apply client --iface <slave-a-iface> --source <master-ip> --ptp --yes
sudo timesync apply client --iface <slave-b-iface> --source <master-ip> --ptp --yes
```

Verification:

```bash
sudo timesync status
sudo timesync guard-ptp
systemctl is-active timesync-ptp-guard.timer
systemctl is-enabled timesync-ptp-guard.timer
```

Troubleshooting commands:

```bash
chronyc sources -v
chronyc tracking
journalctl -u chrony -n 100 --no-pager
journalctl -u ptp4l -n 120 --no-pager
journalctl -u phc2sys -n 80 --no-pager
sudo tcpdump -ni <iface> 'udp port 319 or udp port 320'
```
