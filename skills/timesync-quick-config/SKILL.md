---
name: timesync-quick-config
description: Configure and verify time-sync-cli NTP/PTP roles on robot and industrial Linux hosts, including Darwin VLA master/slave topology and 1970 clock recovery.
---

# timesync-quick-config

Use this skill when a user asks to configure `timesync`, recover a host from a 1970/epoch clock reset, or set up a small NTP/PTP fleet.

## Inputs To Discover

Collect these facts before applying a role:

- SSH alias for each host.
- CPU architecture from `uname -m`.
- PTP-capable interface from `timesync doctor` or `ethtool -T`.
- Master IP on the PTP/NTP subnet.
- Desired role: `master --ptp` or `client --ptp`.

Useful probe:

```bash
ssh <host> 'hostname; uname -m; command -v timesync || true; ip -br addr; sudo timesync doctor; sudo timesync status'
```

## Deploy Current Binary

Build the right binary locally:

```bash
GOOS=linux GOARCH=arm64 go build -o /tmp/timesync-linux-arm64 ./cmd/timesync
GOOS=linux GOARCH=amd64 go build -o /tmp/timesync-linux-amd64 ./cmd/timesync
```

Install on a host:

```bash
scp /tmp/timesync-linux-arm64 <host>:/tmp/timesync-new
ssh <host> 'sudo cp /usr/bin/timesync /usr/bin/timesync.bak-$(date +%Y%m%d%H%M%S) && sudo install -m 0755 /tmp/timesync-new /usr/bin/timesync'
```

If sudo requires a password or a TTY, pause and ask the user for the required access path.

## Apply Roles

NTP + PTP master:

```bash
sudo timesync apply master --iface <master-iface> --ptp --ntp-pool cn.pool.ntp.org --ntp-serve-cidr <ptp-cidr> --yes
```

PTP slave:

```bash
sudo timesync apply client --iface <slave-iface> --source <master-ip> --ptp --yes
```

Darwin VLA target topology:

```bash
# darwin_vla_orin
sudo timesync apply master --iface eth2 --ptp --ntp-pool cn.pool.ntp.org --ntp-serve-cidr 192.168.71.0/24 --yes

# darwin_vla_rt
sudo timesync apply client --iface eth0 --source 192.168.71.51 --ptp --yes

# darwin_vla_5090
sudo timesync apply client --iface enp3s0 --source 192.168.71.51 --ptp --yes
```

## Verify

Run after every apply:

```bash
sudo timesync status
sudo timesync guard-ptp
systemctl is-active timesync-ptp-guard.timer
systemctl is-enabled timesync-ptp-guard.timer
```

Expected PTP slave status:

```text
PTP health: true
Clock health: true
Overall health: true
Configured role: client
Configured PTP: true
Active role: ptp
Source: SLAVE
ptp4l: active
phc2sys: active
```

Expected PTP master status:

```text
NTP health: true
PTP health: true
Clock health: true
Overall health: true
Configured role: master
Configured PTP: true
Source: MASTER
ptp4l: active
phc2sys: active
```

Darwin VLA final verified targets:

```text
darwin_vla_orin  master  eth2    192.168.71.51  grandmaster 90b3d5.fffe.543702
darwin_vla_rt    slave   eth0    192.168.71.13  source 192.168.71.51
darwin_vla_5090  slave   enp3s0  192.168.71.60  source 192.168.71.51
```

## Epoch Recovery

Use this on a host that fell back near 1970:

```bash
sudo timesync repair-clock
sudo timesync status
```

With explicit interface:

```bash
sudo timesync repair-clock --iface <ptp-iface>
```

The generated PTP client units already install boot and runtime guards:

```text
ptp4l.service: ExecStartPre=/usr/bin/timesync boot-guard --repair-system-clock
phc2sys.service: ExecStartPre=/usr/bin/timesync wait-ptp --timeout 30s
timesync-ptp-guard.timer: OnUnitActiveSec=5s
```

## Failure Handling

- Missing `ptp4l`, `phc2sys`, `phc_ctl`, or `pmc`: install `linuxptp`, then apply again.
- Missing hardware timestamping: choose a PTP-capable interface from `timesync doctor`.
- Master NTP failure: inspect `chronyc sources -v` and `chronyc tracking`.
- Slave stays `LISTENING` or `UNCALIBRATED`: inspect `journalctl -u ptp4l -n 120 --no-pager`.
- Suspected PTP packet loss: run `sudo tcpdump -ni <iface> 'udp port 319 or udp port 320'`.
- `phc2sys` convergence issues: inspect `journalctl -u phc2sys -n 80 --no-pager` and confirm generated args include `-S 1.0`.
- `status` shows `unknown`: run `sudo timesync status` so pmc can access the ptp4l management socket.
- Unreachable host: report the exact SSH alias, host, port, and error.
- Sudo requires a password: stop host mutation and ask for a working root path.
