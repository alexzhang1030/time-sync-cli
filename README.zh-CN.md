# time-sync-cli

面向机器人、工控机与嵌入式 Linux 的时间同步 CLI/TUI。通过简单的角色化配置，隐藏 chrony / linuxptp 的复杂细节。

**语言：** [English](README.md) · [简体中文](README.zh-CN.md)

## 功能

| 命令 | 作用 |
|------|------|
| `timesync doctor` | 检测 OS、systemd、依赖二进制、网卡、PTP 硬件时间戳 |
| `timesync status` | 角色感知健康仪表板、纯文本或 JSON 输出 |
| `timesync apply auto\|master\|client` | 应用 NTP/PTP 角色配置，支持 `--dry-run` 和备份 |
| `timesync repair-clock` | 系统时间回到 epoch 后，用 RTC 恢复系统时钟和 PHC |
| `timesync publish-gm-time-properties` | 在 PTP master 上对齐 PHC 并发布有效 GM UTC 偏移 |
| `timesync guard-ptp` | 周期性守卫：管理 `phc2sys`、回写 RTC、恢复 PTP master |
| `timesync tui` | 交互式方向键配置 |

预编译二进制和 `.deb`/`.rpm` 包见 [GitHub Releases](https://github.com/alexzhang1030/time-sync-cli/releases)。

## NTP 与 PTP

| | NTP | PTP |
|---|---|---|
| 时钟 | 系统时钟（`CLOCK_REALTIME`） | 网卡 PHC，再同步到系统时钟 |
| 典型精度 | 毫秒 | 微秒（需硬件时间戳） |
| 上游 | NTP 服务器 / pool | PTP Grandmaster |
| 后端 | chrony | linuxptp（`ptp4l`、`phc2sys`） |
| 特殊网卡 | 否 | 是 |

## 角色

| 角色 | NTP | PTP | 适用场景 |
|------|-----|-----|----------|
| `auto` | 客户端 → 互联网 pool | 可选监测（`--ptp`） | 有公网的边缘设备；不会静默成为 master |
| `master` | 对 CIDR 提供 NTP 服务 | 可选 Grandmaster（`--ptp`） | 工位/子网本地时间源 |
| `client` | 客户端 → `--source` | 可选 Slave（`--ptp`） | 跟随指定上游 |

### 示例

```bash
# auto（安全默认）
sudo timesync apply auto --iface eth0 --ntp-pool pool.ntp.org

# NTP master
sudo timesync apply master --iface eth0 \
  --ntp-pool cn.pool.ntp.org --ntp-serve-cidr 192.168.0.0/24

# PTP Grandmaster
sudo timesync apply master --iface eth0 --ptp \
  --ntp-pool cn.pool.ntp.org --ntp-serve-cidr 192.168.0.0/24

# PTP Slave
sudo timesync apply client --iface eth0 --source 192.168.1.1 --ptp

# 预览变更，不实际执行
timesync apply master --dry-run --iface eth0 --ptp
```

先确认 PTP 硬件支持：

```bash
timesync doctor
```

## 时钟链路

典型 Linux 设备有三类时钟：

1. **系统时钟**（`CLOCK_REALTIME`）— 墙钟时间
2. **网卡 PHC** — 网卡上的 PTP 硬件时钟
3. **RTC** — 主板电池备份时钟

`timesync` 的协调方式：

- **NTP 角色：** chrony 校准系统时钟，并通过 `rtcsync` 周期性写回 RTC。
- **PTP Master：** chrony 提供可信系统时间；`phc2sys` 以系统时钟为参考驯服 PHC；`ptp4l` 把 PHC 提供给客户端。
- **PTP Client：** `ptp4l` 在 PHC 上跟随 Grandmaster；`phc2sys` 以 PHC 为参考驯服系统时钟。

在 PTP Master 上，`timesync` 只在 `phc2sys` 把 PHC 对齐到 `System + TAI–UTC` 后，才会发布 `currentUtcOffsetValid=1`。以下场景会自动完成发布：

- `timesync repair-clock`（master）
- `timesync publish-gm-time-properties`
- 周期性 `timesync guard-ptp` 检测到 valid 位丢失时

## 状态查看

```bash
timesync status                  # TTY 仪表板
timesync status --output plain   # 日志/管道稳定纯文本
timesync status --output json    # 结构化输出
```

报告包含：已配置角色、时钟来源链路、NTP/PTP 健康度、systemd unit 状态、PHC/RTC 残差。

## 从 epoch 时钟回退中恢复

当系统时钟回到 ~1970 时，用 RTC 快速启动，再交给 PTP 收敛：

```bash
sudo timesync repair-clock
# 或显式指定网卡
sudo timesync repair-clock --iface eth0
```

PTP Master 还会额外等待 PHC 对齐并发布 GM 时间属性。

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

1. **`doctor`** — 检测 OS、systemd、二进制、网卡、`ethtool -T` PTP 能力。
2. **`apply --dry-run`** — 按角色渲染 chrony/ptp4l/phc2sys 配置和 systemd unit。
3. **`apply`** — 备份已有文件，写入配置，保存 `state.json`，重载 systemd，重启服务。
4. **`status`** — 读取 systemd、`chronyc`、`pmc`、PHC/RTC 采样和 `state.json`。
5. **`guard-ptp`** — 每 5 秒运行一次，保持 `phc2sys` 状态与健康度一致。

生成文件：

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

## 命令

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

`apply` 需要 root，覆盖前会自动备份。覆盖非 `timesync` 创建的文件时，TTY 会提示确认，CI 中需加 `--yes`。

## 安装

### 预编译二进制

```bash
curl -fsSL -o timesync https://github.com/alexzhang1030/time-sync-cli/releases/latest/download/timesync-linux-amd64
chmod +x timesync
sudo install -m 0755 timesync /usr/bin/timesync
```

ARM64 把 `amd64` 换成 `arm64`。

### 发行版包

```bash
# Debian/Ubuntu
curl -fsSL https://raw.githubusercontent.com/alexzhang1030/time-sync-cli/main/scripts/install-deb.sh | bash

# 卸载
curl -fsSL https://raw.githubusercontent.com/alexzhang1030/time-sync-cli/main/scripts/uninstall.sh | bash
```

Tagged release 同时提供 `.deb` 和 `.rpm` 产物。

### 从源码构建

```bash
go build -o timesync ./cmd/timesync
sudo install -m 0755 timesync /usr/bin/timesync
```

## 依赖

- 带 systemd 的 Linux
- `chrony`
- `linuxptp`（`ptp4l`、`phc2sys`），PTP 角色需要
- `ethtool`，用于 PTP 硬件能力检测

## 开发

```bash
go test ./...
go build -o timesync ./cmd/timesync
```

## 许可证

MIT
