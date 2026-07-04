# time-sync-cli

面向机器人、工控机与嵌入式 Linux 的时间同步 CLI/TUI。通过简单的角色化配置，隐藏 chrony / linuxptp 的复杂细节。

**语言：** [English](README.md) · [简体中文](README.zh-CN.md)

## 当前已实现

| 领域 | 能力 |
|------|------|
| 检测 | `timesync doctor` — OS、systemd、依赖二进制、网卡、通过 `ethtool -T` 检测 PTP 硬件时间戳 |
| 状态 | `timesync status` — 已配置角色、NTP/PTP 偏移与源、时钟健康度、端口状态、路径延迟、systemd unit 状态 |
| 配置 | `timesync apply auto\|master\|client`，支持 `--dry-run`、可选 `--ptp`、文件备份、`--yes` 确认覆盖 |
| 交互配置 | `timesync tui` — 方向键菜单（doctor/status/apply）；非 TTY 时回退为编号问答 |
| RTC 回写 | chrony 配置中的 `rtcsync`；PTP runtime guard 将可信系统时间写回 RTC |
| 时钟修复 | `timesync repair-clock` — 系统时间回到 epoch 后，用 RTC 快速恢复系统时钟和 PHC |
| 发布 | [GitHub Releases](https://github.com/alexzhang1030/time-sync-cli/releases) 提供 `linux/amd64`、`linux/arm64` 预编译包及 `.deb`/`.rpm` |

## NTP 和 PTP 是什么？

### NTP（Network Time Protocol，网络时间协议）

NTP 通过 IP 网络同步**系统时钟**。设备向上游 NTP 服务器（或本地 NTP 服务器）查询当前时间，估算网络延迟，并逐步调整系统时钟。

典型精度：**毫秒级**（局域网常见 1–50 ms，公网更大）。

适用场景：

- 需要正确墙钟时间的一般 Linux 主机
- 间歇性联网的设备
- 毫秒级精度已足够的场景

`timesync` 通过 **chrony**（`chronyd` / `chronyc`）管理 NTP。

### PTP（Precision Time Protocol，精确时间协议，IEEE 1588）

PTP 在**网卡 / PHC（PTP 硬件时钟）**层同步时间，依赖支持硬件时间戳的网卡。Grandmaster 发布时间，Slave 跟随，精度远高于 NTP。

典型精度：**亚微秒到数十微秒**（需硬件时间戳）。

适用场景：

- 机器人集群、运动控制、工业相机、激光雷达/毫米波融合
- 确定性局域网拓扑（单广播域或 PTP 感知交换机）
- 需要微秒级对齐的链路

`timesync` 通过 **linuxptp**（`ptp4l`、`phc2sys`）管理 PTP。

生成的 PTP systemd unit 会在 `ptp4l` 启动前运行 boot guard。PTP client unit 会用 RTC 修复 epoch 系统时间，再把系统时间写入网卡 PHC。当系统时间和 RTC 都像正常年份但相差超过 1 小时时，PTP 会 fail closed。PTP master 和 auto unit 会要求系统时间可信，再初始化 PHC。PTP client unit 还会在 `phc2sys` 启动前等待 PTP 健康，确保 `ptp4l` 已经处于健康 slave 状态且 offset 受控后，PHC 到系统时钟同步可以修复异常系统时间。

### NTP 与 PTP 对比

| | NTP | PTP |
|---|---|---|
| 主要时钟 | 系统时钟（CLOCK_REALTIME） | 网卡 PHC，再同步到系统时钟 |
| 典型精度 | 毫秒 | 微秒（需硬件 TS） |
| 上游 | NTP 服务器 / pool | PTP Grandmaster |
| 后端 | chrony | linuxptp |
| 是否需要特殊网卡 | 否 | 是（硬件 PTP） |

## 角色：主（Master）与从（Client / Slave）

`timesync` 提供三种角色。**Master** 与 **client** 需显式指定；**auto** 是联网设备的安全默认。

| 角色 | NTP 行为 | PTP 行为 | 适用场景 |
|------|----------|----------|----------|
| `auto` | NTP 客户端 → 互联网 pool | 可选 PTP 监测（`--ptp` 且硬件支持） | 有公网的边缘设备；不会静默成为 master |
| `master` | 对 CIDR 提供 NTP 服务 | 可选 PTP Grandmaster（`--ptp`） | 工位/子网本地时间源 |
| `client` | NTP 客户端 → `--source` | 可选 PTP Slave（`--ptp`） | 跟随指定上游 |

### 开启 auto 模式（互联网同步，安全默认）

```bash
# 预览 — NTP 客户端连 pool.ntp.org；加 --ptp 且硬件支持时可启用 PTP 监测
timesync apply auto --dry-run --iface eth0

# 实际应用
sudo timesync apply auto --iface eth0 --ntp-pool pool.ntp.org
sudo timesync apply auto --iface eth0 --ptp   # 硬件支持时同时运行 ptp4l 监测
```

`auto` 不会启用本地 NTP 授时或 PTP Grandmaster — 需要显式 `apply master`。
应用非 PTP 角色时，`timesync` 会停止旧的 timesync 管理 PTP 服务和运行中守卫 timer，防止角色切换后残留的 PHC 到系统时钟同步继续运行。
`auto --ptp` 会保持 chrony 作为唯一系统时钟驯服源，并运行不带 `phc2sys` 的 `ptp4l`，避免 NTP/PTP 同时写 `CLOCK_REALTIME`。

### 开启 NTP 主端（本地授时）

```bash
# 预览
timesync apply master --dry-run --iface eth0 --ntp-pool cn.pool.ntp.org --ntp-serve-cidr 192.168.0.0/24

# 实际应用（需要 root）
sudo timesync apply master --iface eth0 --ntp-pool cn.pool.ntp.org --ntp-serve-cidr 192.168.0.0/24
```

会生成含上游 NTP pool、`local stratum 8` 与 `allow <cidr>` 的 chrony 配置，安装 systemd drop-in，并重启 `chrony`。

### 开启 PTP Grandmaster（Master + PTP）

```bash
sudo timesync apply master --iface eth0 --ptp
```

请先确认硬件时间戳能力：

```bash
timesync doctor   # 查看各网卡 PTP 能力
```

### 开启 NTP 从端（跟随上游）

```bash
sudo timesync apply client --iface eth0 --source 192.168.1.1
```

### 开启 PTP Slave（Client + PTP）

```bash
sudo timesync apply client --iface eth0 --source 192.168.1.1 --ptp
```

PTP 从端在指定 `--source` 时通过 `ptp4l` 单播跟随主站（见生成配置中的 `[unicast_master_table]`）。

### 交互式配置

```bash
timesync tui
```

在交互式终端上，`timesync tui` 会打开全屏菜单：

- **Doctor** — 系统检测（OS、二进制、网卡、PTP 能力）
- **Status** — 同步健康度、角色、偏移、systemd unit 状态
- **Apply** — 引导式角色/网卡配置，支持 dry-run、apply 或 cancel
- **Quit** — 退出

使用 **↑/↓**（或 `j`/`k`）导航，**Enter** 确认，**Esc** 返回。当 stdin 不是 TTY（管道、CI、自动化）时，同样的流程通过编号问答提供。

## 硬件时钟（RTC）同步

典型 Linux 设备上有三类相关时钟：

1. **系统时钟**（`CLOCK_REALTIME`）— 用户与应用看到的时间
2. **网卡 PHC** — 网卡上的 PTP 硬件时钟（仅 PTP 路径）
3. **RTC** — 主板电池备份的硬件时钟

### `timesync` 当前行为

| 路径 | 机制 | 方向 |
|------|------|------|
| NTP（chrony） | 生成配置中的 `rtcsync` | 系统时钟 → RTC（周期性回写） |
| PTP Client（linuxptp） | `phc2sys -s <iface> -w -S 1.0` | PHC -> 系统时钟；`-w` 等待 ptp4l；`-S 1.0` 直接跳正大于 1 秒的初始偏差 |
| PTP Master（linuxptp） | `phc2sys -s CLOCK_REALTIME -c <iface> -w -S 1.0` | 系统时钟 -> PHC；`-w` 等待 ptp4l；`-S 1.0` 直接跳正大于 1 秒的初始偏差 |

同步成功后：

- **NTP 角色：** chrony 校准系统时钟，并通过 `rtcsync` 将修正推送到 RTC。
- **PTP Client 角色：** `timesync` 会停用两种常见 chrony 服务名（`chrony` 和 `chronyd`），等待健康的 `ptp4l` slave 状态，然后 `phc2sys` 以 PHC 为参考驯服系统时钟，并直接跳正大于 1 秒的偏差。运行中守卫会在 PTP、PHC、系统时间互相可信后，把系统时间写回 RTC。
- **PTP Master 角色：** `phc2sys` 以 `CLOCK_REALTIME` 为参考驯服 PHC，然后 `ptp4l` 把这个硬件时钟提供给客户端。

### 验证 RTC / 同步状态

```bash
timesync status           # NTP + PTP 同步健康度、端口状态、偏移、路径延迟
chronyc tracking          # NTP 偏移与参考源
pmc -u -b 0 'GET TIME_STATUS_NP'   # 原始 PTP 偏移（linuxptp）
timedatectl status        # 系统时钟 + RTC 同步标志
```

`timesync status` 也会显示系统/RTC/PHC 的 Unix 时间和偏差。系统时间接近 epoch、RTC 接近 epoch、RTC 与系统时间相差超过 1 小时、或 PHC 与系统时间相差超过 120 秒时，clock health 会变成 false。已配置的 PTP client/master 整体健康要求符合角色预期的 PTP 端口状态和 active `phc2sys`，因此残留 NTP 或 PHC 到系统时钟同步中断无法掩盖已配置 PTP 角色的故障。

### 从 1970 / epoch 时钟回退中恢复

当系统时钟和网卡 PHC 回到 `1970-01-01` 附近时，PTP 可能已经进入 `SLAVE`，同时显示巨大的 `master offset`。先用 RTC 快速启动，再交给 PTP 收敛：

```bash
sudo timesync repair-clock
```

该命令默认使用 `/etc/timesync-cli/state.json` 中上次应用的网卡。需要显式指定网卡时：

```bash
sudo timesync repair-clock --iface eth0
```

等价手动步骤：

```bash
sudo systemctl stop phc2sys ptp4l
sudo date -u -s "@$(cat /sys/class/rtc/rtc0/since_epoch)"
sudo phc_ctl eth0 set
sudo systemctl start ptp4l
sudo systemctl start phc2sys
timesync status
```

生成的 PTP 角色也会把这条预防链路写入 `ptp4l.service`：

```ini
ExecStartPre=/usr/bin/timesync boot-guard --iface eth0 --repair-system-clock
```

PTP master 和 auto 角色使用更严格的授时门禁：

```ini
ExecStartPre=/usr/bin/timesync boot-guard --iface eth0 --require-trusted-system-clock
```

生成的 PTP client 角色也会给 `phc2sys` 增加启动门禁：

```ini
ExecStartPre=/usr/bin/timesync wait-ptp --timeout 30s
```

PTP client 角色还会安装运行中守卫 timer：

```ini
ExecStart=/usr/bin/timesync guard-ptp
OnUnitActiveSec=5s
```

当 PTP 健康度变红时，守卫会保持 `phc2sys` 停止。PTP 健康时，守卫会启动或保持 `phc2sys` 运行，让健康 PHC 修复异常系统时钟。当系统时间与 PHC 接近但 RTC 陈旧时，守卫会把可信系统时间写回 RTC。守卫执行保护动作后会正常退出并记录动作日志，因此周期 timer 在长时间故障期间仍会持续运行。

## 实现原理

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

1. **检测（`doctor`）** — 读取 `/etc/os-release`，检查 systemd，定位二进制，列出 `/sys/class/net` 网卡，对每块网卡执行 `ethtool -T` 检测 PTP 硬件时间戳。
2. **规划（`apply --dry-run`）** — 按角色渲染 chrony/ptp4l/phc2sys 配置、chrony drop-in 和 PTP systemd unit。
3. **应用（`apply` 无 `--dry-run`）** — 备份已有文件，写入配置，保存 `state.json`，执行 `systemctl daemon-reload`，enable 并 restart 相关 unit。
4. **状态（`status`）** — 只读：`systemctl is-active`、`chronyc -c tracking`、从 `state.json` 读取已配置角色。

### 生成目录结构

```
/etc/timesync-cli/
├── chrony.conf          # NTP 客户端或服务端配置
├── ptp4l.conf           # PTP Grandmaster 或 Slave（启用 --ptp 时）
├── phc2sys.conf         # phc2sys 参考参数（说明性）
├── state.json           # 上次应用的角色
└── backups/             # 覆盖前的时间戳备份

/etc/systemd/system/
├── chrony.service.d/timesync-cli.conf
├── ptp4l.service
├── phc2sys.service
├── timesync-ptp-guard.service
└── timesync-ptp-guard.timer
```

## 依赖

- 带 systemd 的 Linux
- `chrony`（chronyd / chronyc）
- `linuxptp`（ptp4l、phc2sys），PTP 角色需要
- `ethtool`，用于 PTP 硬件能力检测

## 安装

### 预编译二进制

从[最新 Release](https://github.com/alexzhang1030/time-sync-cli/releases/latest) 下载：

| 平台 | 产物 |
|------|------|
| Linux x86_64（`linux/amd64`） | [`timesync-linux-amd64`](https://github.com/alexzhang1030/time-sync-cli/releases/latest/download/timesync-linux-amd64) |
| Linux ARM64（`linux/arm64`） | [`timesync-linux-arm64`](https://github.com/alexzhang1030/time-sync-cli/releases/latest/download/timesync-linux-arm64) |

```bash
# 示例：amd64
curl -fsSL -o timesync https://github.com/alexzhang1030/time-sync-cli/releases/latest/download/timesync-linux-amd64
chmod +x timesync
sudo install -m 0755 timesync /usr/bin/timesync
```

### 发行版包（`.deb`、`.rpm`）

带 tag 的 release 提供 `linux/amd64` 与 `linux/arm64` 原生包：

Debian/Ubuntu 一键安装：

```bash
curl -fsSL https://raw.githubusercontent.com/alexzhang1030/time-sync-cli/main/scripts/install-deb.sh | bash
```

一键卸载：

```bash
curl -fsSL https://raw.githubusercontent.com/alexzhang1030/time-sync-cli/main/scripts/uninstall.sh | bash
```

一键清理 NTP/PTP 角色配置：

```bash
curl -fsSL https://raw.githubusercontent.com/alexzhang1030/time-sync-cli/main/scripts/uninstall-config.sh | bash
```

安装指定版本：

```bash
curl -fsSL https://raw.githubusercontent.com/alexzhang1030/time-sync-cli/main/scripts/install-deb.sh | bash -s -- v0.2.5
```

| 格式 | 示例产物 |
|------|----------|
| Debian/Ubuntu（`.deb`） | `timesync_<version>_amd64.deb` |
| RHEL/Fedora（`.rpm`） | `timesync-<version>-1.x86_64.rpm` |

```bash
# Debian/Ubuntu（amd64）
curl -fsSLO https://github.com/alexzhang1030/time-sync-cli/releases/latest/download/timesync_<version>_amd64.deb
sudo apt install ./timesync_<version>_amd64.deb

# RHEL/Fedora（amd64）
curl -fsSLO https://github.com/alexzhang1030/time-sync-cli/releases/latest/download/timesync-<version>-1.x86_64.rpm
sudo dnf install ./timesync-<version>-1.x86_64.rpm
```

包将二进制安装到 `/usr/bin/timesync`，并声明运行时依赖 `chrony`、`ethtool`（推荐 `linuxptp` 用于 PTP 角色）。

卸载脚本会停止 timesync 管理的 PTP 服务，删除 timesync systemd 文件和 `/etc/timesync-cli`，再卸载 `timesync` 包。`chrony`、`linuxptp`、`ethtool` 等运行时依赖包会保留。

### 从源码构建

```bash
go build -o timesync ./cmd/timesync
sudo install -m 0755 timesync /usr/bin/timesync
```

## 命令

```bash
timesync doctor                                          # 检测 OS、工具、网卡、PTP 能力
timesync status                                          # 同步健康度、角色、NTP/PTP 偏移、端口状态
timesync apply auto [--iface eth0] [--ntp-pool pool.ntp.org] [--ptp] [--dry-run] [--yes]
timesync apply master --iface eth0 [--ptp] [--ntp-serve-cidr 192.168.0.0/24] [--dry-run] [--yes]
timesync apply client --iface eth0 --source <host> [--ptp] [--dry-run] [--yes]
sudo timesync repair-clock [--iface eth0]                 # 系统时间回到 epoch 后从 RTC 恢复系统时钟和 PHC
timesync uninstall [--dry-run] [--yes]                     # 清理 timesync 管理的 NTP/PTP 角色配置
timesync tui                                             # 交互式引导配置
timesync rollback                                        # 从上次 apply 备份恢复
```

不带 `--dry-run` 的 apply 需要 root（`sudo`），将：

- 写入 `/etc/timesync-cli/` 下配置
- 备份目标文件到 `/etc/timesync-cli/backups/`
- 安装 systemd drop-in 并重启相关服务

### 覆盖确认

当目标配置文件已存在、但**不是** timesync 创建的（既无 `state.json` 记录，也无 `timesync-cli` 标记），apply 在覆盖前会先确认：

- **交互式终端：** 列出受影响的文件并提示 `y/N`。
- **非交互 / CI：** 拒绝执行并报错，除非传入 `--yes`。
- **`--yes`：** 跳过提示直接覆盖（覆盖前仍会备份原内容）。

```bash
# CI / 脚本化 apply，可能覆盖手写配置
sudo timesync apply client --iface eth0 --source 192.168.1.1 --yes
```

### 清理当前 NTP/PTP 角色配置

```bash
timesync uninstall --dry-run
sudo timesync uninstall --yes
```

这会删除 timesync 管理的 NTP/PTP Master/Client 配置，包括 `/etc/timesync-cli`、timesync systemd drop-in、timesync 创建的 `ptp4l` / `phc2sys` unit，以及 PTP guard timer。`timesync` 二进制和运行时依赖包会保留。

一键清理：

```bash
curl -fsSL https://raw.githubusercontent.com/alexzhang1030/time-sync-cli/main/scripts/uninstall-config.sh | bash
```

## 安全模型

- 配置生成在 `/etc/timesync-cli/` — 不直接修改发行版 chrony/ptp4l 文件。
- 通过 systemd drop-in 覆盖 unit 行为。
- `--dry-run` 预览变更，无需 root。
- 实际应用需要 `sudo`，覆盖前自动备份。
- 覆盖非 timesync 创建的文件需确认（TTY 上 `y/N`，CI 中用 `--yes`）。
- `auto` 不会启用本地授时；需显式 `apply master`。
- PTP 需要硬件时间戳 — 用 `timesync doctor` 确认。

## 路线图 / 尚未实现

| 功能 | 状态 |
|------|------|
| CI 矩阵构建产物（`linux/amd64`、`linux/arm64`） | 已完成 — 见 [releases](https://github.com/alexzhang1030/time-sync-cli/releases) |
| 发行版打包（`.deb`、`.rpm`） | 已完成 |
| PTP 单播客户端（`--source` → ptp4l unicast master） | 已完成 |
| apply 前自动检测 PTP 硬件再启用 `--ptp` | 已完成 |
| 覆盖非 timesync 配置前的交互确认 | 已完成 |
| `timesync rollback` 恢复备份 | 已完成 |
| 集群选主（避免多 master） | 范围外（设计如此） |
| 富 TUI（方向键菜单） | 已完成 |
| 深度 PTP 状态解析（端口状态、偏移） | 已完成 |

## 开发

```bash
go test ./...
go build -o timesync ./cmd/timesync
```

## 支持的假设

- systemd 初始化
- chrony 负责 NTP 客户端/服务端
- linuxptp 负责 PTP Grandmaster/Slave
- 网卡通过 `/sys/class/net` 暴露

## 许可证

MIT
