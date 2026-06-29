# time-sync-cli

面向机器人、工控机与嵌入式 Linux 的时间同步 CLI/TUI。通过简单的角色化配置，隐藏 chrony / linuxptp 的复杂细节。

**语言：** [English](README.md) · [简体中文](README.zh-CN.md)

## 当前已实现

| 领域 | 能力 |
|------|------|
| 检测 | `timesync doctor` — OS、systemd、依赖二进制、网卡、通过 `ethtool -T` 检测 PTP 硬件时间戳 |
| 状态 | `timesync status` — 已配置角色、NTP 偏移/源、systemd unit 状态 |
| 配置 | `timesync apply auto\|master\|client`，支持 `--dry-run`、可选 `--ptp`、文件备份 |
| 交互配置 | `timesync tui` — stdin 问答选择角色、网卡及 apply/dry-run/cancel |
| RTC 回写 | chrony 配置中的 `rtcsync`；PTP drop-in 中的 `phc2sys -w` |
| 发布 | [GitHub Releases](https://github.com/alexzhang1030/time-sync-cli/releases) 提供 `linux/amd64`、`linux/arm64` 预编译包 |

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
| `auto` | NTP 客户端 → 互联网 pool | 可选 PTP 从端（`--ptp` 且硬件支持） | 有公网的边缘设备；不会静默成为 master |
| `master` | 对 CIDR 提供 NTP 服务 | 可选 PTP Grandmaster（`--ptp`） | 工位/子网本地时间源 |
| `client` | NTP 客户端 → `--source` | 可选 PTP Slave（`--ptp`） | 跟随指定上游 |

### 开启 auto 模式（互联网同步，安全默认）

```bash
# 预览 — NTP 客户端连 pool.ntp.org；加 --ptp 且硬件支持时可启用 PTP
timesync apply auto --dry-run --iface eth0

# 实际应用
sudo timesync apply auto --iface eth0 --ntp-pool pool.ntp.org
sudo timesync apply auto --iface eth0 --ptp   # 硬件支持时同时启用 PTP 从端
```

`auto` 不会启用本地 NTP 授时或 PTP Grandmaster — 需要显式 `apply master`。

### 开启 NTP 主端（本地授时）

```bash
# 预览
timesync apply master --dry-run --iface eth0 --ntp-serve-cidr 192.168.0.0/24

# 实际应用（需要 root）
sudo timesync apply master --iface eth0 --ntp-serve-cidr 192.168.0.0/24
```

会生成含 `local stratum 8` 与 `allow <cidr>` 的 chrony 配置，安装 systemd drop-in，并重启 `chronyd`。

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

PTP 从端通过 `ptp4l` 在 L2 域内发现/跟随 Grandmaster；`--source` 预留给后续单播 PTP 目标。

### 交互式配置

```bash
timesync tui
```

## 硬件时钟（RTC）同步

典型 Linux 设备上有三类相关时钟：

1. **系统时钟**（`CLOCK_REALTIME`）— 用户与应用看到的时间
2. **网卡 PHC** — 网卡上的 PTP 硬件时钟（仅 PTP 路径）
3. **RTC** — 主板电池备份的硬件时钟

### `timesync` 当前行为

| 路径 | 机制 | 方向 |
|------|------|------|
| NTP（chrony） | 生成配置中的 `rtcsync` | 系统时钟 → RTC（周期性回写） |
| PTP（linuxptp） | `phc2sys -s <iface> -w` | PHC → 系统时钟；`-w` 在大步调整时写入 RTC |

同步成功后：

- **NTP 角色：** chrony 校准系统时钟，并通过 `rtcsync` 将修正推送到 RTC。
- **PTP 角色：** `phc2sys` 以 PHC 为参考驯服系统时钟；带 `-w` 时大步调整会传播到 RTC。

### 验证 RTC / 同步状态

```bash
timesync status
chronyc tracking          # NTP 偏移与参考源
timedatectl status        # 系统时钟 + RTC 同步标志
```

## 实现原理

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

1. **检测（`doctor`）** — 读取 `/etc/os-release`，检查 systemd，定位二进制，列出 `/sys/class/net` 网卡，对每块网卡执行 `ethtool -T` 检测 PTP 硬件时间戳。
2. **规划（`apply --dry-run`）** — 按角色渲染 chrony/ptp4l/phc2sys 配置与 systemd drop-in，写入 `/etc/timesync-cli/`，不直接修改发行版自带配置。
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
├── chronyd.service.d/timesync-cli.conf
├── ptp4l.service.d/timesync-cli.conf
└── phc2sys.service.d/timesync-cli.conf
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
sudo mv timesync /usr/local/bin/
```

### 从源码构建

```bash
go build -o timesync ./cmd/timesync
sudo mv timesync /usr/local/bin/
```

## 命令

```bash
timesync doctor                                          # 检测 OS、工具、网卡、PTP 能力
timesync status                                          # 同步健康度、角色、源、偏移
timesync apply auto [--iface eth0] [--ntp-pool pool.ntp.org] [--ptp] [--dry-run]
timesync apply master --iface eth0 [--ptp] [--ntp-serve-cidr 192.168.0.0/24] [--dry-run]
timesync apply client --iface eth0 --source <host> [--ptp] [--dry-run]
timesync tui                                             # 交互式引导配置
```

不带 `--dry-run` 的 apply 需要 root（`sudo`），将：

- 写入 `/etc/timesync-cli/` 下配置
- 备份目标文件到 `/etc/timesync-cli/backups/`
- 安装 systemd drop-in 并重启相关服务

## 安全模型

- 配置生成在 `/etc/timesync-cli/` — 不直接修改发行版 chrony/ptp4l 文件。
- 通过 systemd drop-in 覆盖 unit 行为。
- `--dry-run` 预览变更，无需 root。
- 实际应用需要 `sudo`，覆盖前自动备份。
- `auto` 不会启用本地授时；需显式 `apply master`。
- PTP 需要硬件时间戳 — 用 `timesync doctor` 确认。

## Synara 项目

本仓库已注册为 Synara 代码项目：

- **工作区：** `/Users/alex/company/standard/time-sync-cli`
- **元数据：** [`.synara/project.toml`](.synara/project.toml)

在 Synara 中添加/打开指向上述路径的项目（或 clone 到该路径）。默认 thread 环境模式：`worktree`。

## 路线图 / 尚未实现

| 功能 | 状态 |
|------|------|
| CI 矩阵构建产物（`linux/amd64`、`linux/arm64`） | 已完成 — 见 [releases](https://github.com/alexzhang1030/time-sync-cli/releases) |
| 发行版打包（`.deb`、`.rpm`） | 计划中 |
| PTP 单播客户端（`--source` → ptp4l unicast master） | 计划中 |
| apply 前自动检测 PTP 硬件再启用 `--ptp` | 计划中 |
| 覆盖非 timesync 配置前的交互确认 | 计划中 |
| `timesync rollback` 恢复备份 | 计划中 |
| 集群选主（避免多 master） | 范围外（设计如此） |
| 富 TUI（方向键菜单） | 计划中 |
| 深度 PTP 状态解析（端口状态、偏移） | 计划中 |

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
