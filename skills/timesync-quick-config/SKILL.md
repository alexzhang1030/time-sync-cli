---
name: timesync-quick-config
description: Configure and verify time-sync-cli NTP/PTP roles on robot and industrial Linux hosts, including master/slave PTP topology and 1970 clock recovery.
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

Fleet target topology template:

```bash
# master-host
sudo timesync apply master --iface <master-iface> --ptp --ntp-pool cn.pool.ntp.org --ntp-serve-cidr <ptp-cidr> --yes

# slave-a
sudo timesync apply client --iface <slave-a-iface> --source <master-ip> --ptp --yes

# slave-b
sudo timesync apply client --iface <slave-b-iface> --source <master-ip> --ptp --yes
```

## Re-apply an Existing PTP Master After Upgrade

Read the previously applied interface, NTP pool, and served CIDR:

```bash
sudo awk -F'"' '/"iface"/ {print $4}' /etc/timesync-cli/state.json
sudo awk '$1 == "pool" || $1 == "allow" {print}' /etc/timesync-cli/chrony.conf
```

Install the updated generated configuration and systemd unit with the same values:

```bash
sudo timesync apply master --iface <master-iface> --ptp --ntp-pool <existing-pool> --ntp-serve-cidr <existing-cidr> --yes
sudo pmc -u -b 0 'GET GRANDMASTER_SETTINGS_NP'
sudo timesync status
```

Require `currentUtcOffset 37`, `currentUtcOffsetValid 1`, and `ptpTimescale 1` in the management response. The generated `phc2sys.service` publishes these properties after the PHC reaches PTP time scale, and the runtime guard restores them after a later `ptp4l` restart.

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

Final verified target template:

```text
master-host  master  <master-iface>   <master-ip>  grandmaster <gm-identity>
slave-a      slave   <slave-a-iface>  <slave-a-ip> source <master-ip>
slave-b      slave   <slave-b-iface>  <slave-b-ip> source <master-ip>
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
