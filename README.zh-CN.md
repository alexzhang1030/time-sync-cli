# time-sync-cli

面向机器人、工控机与嵌入式 Linux 的时间同步 CLI/TUI。通过简单的角色化配置，隐藏 chrony / linuxptp 的复杂细节。

**语言：** [English](README.md) · [简体中文](README.zh-CN.md)

## 当前已实现

| 领域 | 能力 |
|------|------|
| 检测 | `timesync doctor` — OS、systemd、依赖二进制、网卡、通过 `ethtool -T` 检测 PTP 硬件时间戳 |
| 状态 | `timesync status` — 角色感知终端仪表板、管道稳定纯文本、JSON `1.2` schema，以及时钟来源/链路、NTP 同步、PTP 链路/精度、时钟驯服、运行守卫和真实 systemd unit 状态 |
| 配置 | `timesync apply auto\|master\|client`，支持 `--dry-run`、可选 `--ptp`、文件备份、`--yes` 确认覆盖 |
| 交互配置 | `timesync tui` — 方向键菜单（doctor/status/apply）并复用状态仪表板；非 TTY 时回退为编号问答 |
| RTC 回写 | chrony 配置中的 `rtcsync`；PTP runtime guard 将可信系统时间写回 RTC |
| 时钟修复 | `timesync repair-clock` — 系统时间回到 epoch 后，用 RTC 快速恢复系统时钟和 PHC；PTP master 还会等待 PHC 对齐并发布有效的 GM 时间属性 |
| GM 属性发布 | `timesync publish-gm-time-properties` — 确保 `phc2sys` 已将 PHC 对齐，再在 PTP Grandmaster 上发布并验证 `currentUtcOffsetValid` |
| 运行守卫 | `timesync guard-ptp` — 周期性守卫，管理 `phc2sys`、回写 RTC，并在 PTP master 丢失 `currentUtcOffsetValid` 时自动恢复 |
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
sudo timesync apply master --iface <master-iface> --ptp \
  --ntp-pool cn.pool.ntp.org \
  --ntp-serve-cidr <ptp-cidr>
```

请先确认硬件时间戳能力：

```bash
timesync doctor   # 查看各网卡 PTP 能力
```

生成的 Grandmaster 会声明当前 TAI–UTC 偏移、保守的未知时钟精度（`0xFE`）以及 NTP 来源类型。PTP profile 以 8 Hz 运行 Sync 与 DelayReq（`logSyncInterval=-3`、`logMinDelayReqInterval=-3`），为伺服提供更密集的样本。

`phc2sys` 把 PHC 校准到 `System + TAI–UTC` 后，`timesync` 会通过管理套接字发布 `currentUtcOffsetValid=1` 并回读验证。`repair-clock` 在 master 上、手动运行 `publish-gm-time-properties`、以及周期性 `guard-ptp` 检测到 valid 位丢失时，都会自动执行这一发布。运行中守卫在 `phc2sys` 已在运行但 PHC 不再对齐时也会重启 `phc2sys`，使 master 在 epoch 启动或系统时间跳变后无需人工干预即可恢复。

已配置 master 角色的主机升级后，先读取当前网卡和 NTP 参数：

```bash
sudo awk -F'"' '/"iface"/ {print $4}' /etc/timesync-cli/state.json
sudo awk '$1 == "pool" || $1 == "allow" {print}' /etc/timesync-cli/chrony.conf
```

使用相同参数重新 apply，让新版 `ptp4l.conf` 和 systemd unit 生效：

```bash
sudo timesync apply master --iface <master-iface> --ptp \
  --ntp-pool <existing-pool> \
  --ntp-serve-cidr <existing-cidr> \
  --yes

sudo pmc -u -b 0 'GET GRANDMASTER_SETTINGS_NP'
sudo timesync status
```

管理响应应包含 `currentUtcOffset 37`、`currentUtcOffsetValid 1` 和 `ptpTimescale 1`。下游客户端随后会显示 `PHC as UTC`、数值形式的 `PHC residual` 以及按角色判定的时钟健康度。

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
| PTP Client（linuxptp） | `phc2sys -s <iface> -w -R 8 -N 5 -S 1.0` | PHC -> 系统时钟；`-R 8` 每秒更新八次；`-N 5` 从五次 PHC 读取中选取最快值；`-S 1.0` 直接跳正大于 1 秒的初始偏差 |
| PTP Master（linuxptp） | `phc2sys -s CLOCK_REALTIME -c <iface> -w -S 1.0` | 系统时钟 -> PHC；`-w` 等待 ptp4l；`-S 1.0` 直接跳正大于 1 秒的初始偏差 |

同步成功后：

- **NTP 角色：** chrony 校准系统时钟，并通过 `rtcsync` 将修正推送到 RTC。
- **PTP Client 角色：** `timesync` 会停用两种常见 chrony 服务名（`chrony` 和 `chronyd`），等待健康的 `ptp4l` slave 状态，然后 `phc2sys` 以 PHC 为参考驯服系统时钟，并直接跳正大于 1 秒的偏差。运行中守卫会在 PTP、PHC、系统时间互相可信后，把系统时间写回 RTC。
- **PTP Master 角色：** `phc2sys` 以 `CLOCK_REALTIME` 为参考驯服 PHC，然后 `ptp4l` 把这个硬件时钟提供给客户端。

### 验证 RTC / 同步状态

```bash
timesync status                  # TTY 中显示彩色仪表板
timesync status --output plain   # 日志与管道使用稳定纯文本
timesync status --output json    # 自动化使用结构化输出
chronyc tracking          # NTP 偏移与参考源
pmc -u -b 0 'GET TIME_STATUS_NP'   # 原始 PTP 偏移（linuxptp）
timedatectl status        # 系统时钟 + RTC 同步标志
```

`timesync status` 在交互终端中选择仪表板，stdout 重定向时选择纯文本。`--output auto|fancy|plain|json` 可显式指定格式，`NO_COLOR` 会保留无颜色的仪表板布局。报告会分别展示角色配置、观测到的时钟写入者、来源链路、服务/链路状态、测量精度、时钟有效性、驯服链路和运行守卫状态。

仪表板按各角色的时钟写入契约判定健康度：

| 角色 | 系统时钟来源与链路 | 必需健康项 |
|------|--------------------|------------|
| `auto` | NTP → 系统；PTP 作为可选监测 | NTP 同步、系统时钟 |
| `master --ptp` | NTP → 系统 → PHC → PTP 客户端 | NTP 同步、PTP `MASTER`、系统时钟、`phc2sys`、守卫 |
| `client --ptp` | PTP Grandmaster → PHC → 系统 | PTP `SLAVE`、PTP 精度、系统时钟、`phc2sys`、守卫 |

健康状态分为 `healthy`、`degraded`、`unhealthy`、`unknown`、`disabled` 和 `unmanaged`。NTP 当前修正量在 100 ms 内为 healthy，1 s 内为 degraded；PTP Grandmaster 偏移和归一化 PHC 残差在 10 ms 内为 healthy，1 s 内为 degraded；epoch 时钟和超过 1 小时的 RTC/系统差值为 unhealthy。查询失败映射为 `unknown`，用于区分证据缺失和已测量故障。

PTP 硬件时钟通常采用 TAI 时间尺度，Linux 系统时钟显示 UTC，具体模型参见 [linuxptp `phc2sys` 时钟时间尺度文档](https://www.linuxptp.org/documentation/phc2sys/)。`status` 从 `TIME_PROPERTIES_DATA_SET` 动态读取 `currentUtcOffset` 及其有效位，把 PHC 采样转换为 UTC，再分别报告 `PHC raw (TAI)`、`PHC as UTC` 和 `PHC residual = System − PHC(UTC)`。系统支持时，residual 使用内核关联的 `phc_ctl cmp` 样本，并在 `resolution` 中报告当前采样方式。接近当前 TAI–UTC 差值的原始差值属于预期结果。JSON 保留 `phc_system_skew` 和 `phc_unix` 原始兼容字段，并增加 `phc_utc_unix`、`phc_residual_ns`、`phc_time_scale`、`tai_utc_offset` 和 `tai_utc_offset_valid`，供自动化读取准确语义。

JSON 输出使用增量 schema `1.2`。原有顶层字段 `healthy`、`ntp_health`、`ptp_health`、`clock_health`、`role`、`source` 和 `offset` 继续可用。新消费方推荐使用 `health.*`、`system_clock_source`、`clock_flow`、`management_state`、归一化时钟字段和结构化 systemd unit 记录。

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

PTP client 和 master 角色会把这条恢复链路写入 `ptp4l.service`：

```ini
ExecStartPre=/usr/bin/timesync boot-guard --iface eth0 --repair-system-clock
```

PTP master 角色会在授时前通过 RTC 修复陈旧系统时间，等待 `phc2sys` 将 PHC 对齐到 TAI，然后发布已验证的 GM 时间属性，让下游客户端看到有效的 UTC 偏移：

```ini
ExecStartPre=/usr/bin/timesync boot-guard --iface eth0 --repair-system-clock
ExecStartPost=/usr/bin/timesync publish-gm-time-properties --timeout 30s
```

`publish-gm-time-properties` 也可以在 `ptp4l` 或 `phc2sys` 重启后手动运行：

```bash
sudo timesync publish-gm-time-properties --timeout 30s
```

Auto PTP 监测会在 `ptp4l` 启动前要求可信系统时间：

```ini
ExecStartPre=/usr/bin/timesync boot-guard --iface eth0 --require-trusted-system-clock
```

生成的 PTP client 角色也会给 `phc2sys` 增加启动门禁：

```ini
ExecStartPre=/usr/bin/timesync wait-ptp --timeout 30s
```

PTP client 和 master 角色会安装运行中守卫 timer：

```ini
ExecStart=/usr/bin/timesync guard-ptp
OnUnitActiveSec=5s
```

当 PTP 健康度变红时，守卫会保持 `phc2sys` 停止。PTP 健康时，守卫会启动或保持 `phc2sys` 运行，让健康 PHC 修复异常系统时钟。当系统时间与 PHC 接近但 RTC 陈旧时，守卫会把可信系统时间写回 RTC。

在 PTP master 上，如果 `currentUtcOffsetValid` 变为 false，守卫会执行完整恢复：清除 systemd 陈旧 failed 状态、确保 `chrony` 运行、启动或重启 `phc2sys`、等待 PHC 残差进入 1 秒以内，然后再次发布 GM 时间属性。守卫执行保护动作后会正常退出并记录动作日志，因此周期 timer 在长时间故障期间仍会持续运行。

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
4. **状态（`status`）** — 只读：通过 `systemctl show` 获取真实 unit/load/active/enable 状态，读取 `chronyc -c tracking`、`pmc` port/current/time/time-properties 数据集、PHC/RTC 采样，以及 `state.json` 中的已配置角色。

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
timesync status                                          # 角色感知健康度、来源链路、NTP/PTP 精度、守卫状态
timesync apply auto [--iface eth0] [--ntp-pool pool.ntp.org] [--ptp] [--dry-run] [--yes]
timesync apply master --iface eth0 [--ptp] [--ntp-serve-cidr 192.168.0.0/24] [--dry-run] [--yes]
timesync apply client --iface eth0 --source <host> [--ptp] [--dry-run] [--yes]
sudo timesync repair-clock [--iface eth0]                 # 系统时间回到 epoch 后从 RTC 恢复系统时钟和 PHC；master 还会对齐 PHC 并发布 GM 属性
sudo timesync publish-gm-time-properties [--config /etc/timesync-cli/ptp4l.conf] [--timeout 30s]  # 在 master 上对齐 PHC 并发布 GM UTC 偏移
sudo timesync guard-ptp                                    # 单次执行周期性 PTP 运行守卫
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
