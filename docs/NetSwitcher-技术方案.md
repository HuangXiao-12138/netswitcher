# NetSwitcher 内外网路由管理工具 — 技术实现方案

> 本文档是给 code 智能体的实现规格（spec），目标是按 phase 顺序执行即可交付一个可用的 Windows 桌面应用。
> 所有技术决策已锁定，agent 不应擅自更换框架/语言；如遇阻塞按"风险与边界"章节处理。

---

> ## ⚠️ 实现演化说明（以代码和 README 为准）
>
> 本文档描述的是**原始设计**（Phase 0–7：Windows 服务 + 命名管道 IPC + 非提权 GUI）。
> 实际实现经过用户反馈后**演化成了不同架构**，下方文档内容仅保留作设计参考。
> 当前真实架构请以 **README.md** 和 **代码** 为准。关键差异：
>
> | 维度 | 文档原文（已过时） | 实际实现 |
> |---|---|---|
> | 进程模型 | Windows 服务（SYSTEM）+ 非提权 GUI，命名管道 IPC | **单进程**：提权 GUI 内嵌路由引擎（`appapi` 持有 `*core.Core` 直接调用，不走 IPC） |
> | 首次使用 | `service install`（注册 SCM） | 检测非提权 → 弹"以管理员身份重启" |
> | 开机自启 | SCM 自动启动 | **任务计划**（`schtasks /SC ONLOGON /RL HIGHEST`） |
> | 关窗口 | 服务继续在后台 | GUI 缩到**系统托盘**（fyne.io/systray），进程继续维护路由 |
> | 单实例 | 无 | 命名 mutex（`Local\`）+ 事件信号 |
> | 子进程清理 | 无 | Job Object（`KILL_ON_JOB_CLOSE`），父死子灭 |
> | 窗口 | 原生标题栏 | **Frameless** 自绘顶栏（`--wails-draggable: drag`） |
> | 构建标签 | 无特殊 | `-tags desktop,production` + `-ldflags "-H windowsgui"` |
> | 子进程控制台 | 默认 | 每个子进程 `CREATE_NO_WINDOW`（不闪黑窗） |
> | PowerShell | `ConvertTo-Json -AsArray` | 去掉 `-AsArray`（PS 5.1 不支持）+ GBK 解码中文接口名 |
> | 服务代码 | 核心 | 仍保留（`internal/service`、`internal/ipc`、`internal/app`），但 **GUI 不再使用**（遗留） |
>
> 其余设计（路由引擎声明式 reconcile、配置校验规则、netwatch 轮询去抖、metric 管理、
> 冲突检测、state.json 做 diff 基线、route.exe 子进程 + GBK + 幂等）**与文档一致，已按文档实现**。

---

## 1. 项目概述

**解决的问题**：Windows 机器同时连着内网（以太网，无 Internet）和外网（Wi-Fi，有 Internet）时，让指定网段（如 `168.168.0.0/16` 内网服务器、`172.16.0.0/16` 内网终端）走以太网，其余流量（含默认路由）走 Wi-Fi。手动用 `route -p` 配置的问题是：Wi-Fi 重连、网卡 interface index 变化、DHCP 续约后持久路由经常失效。

**核心定位**：一个常驻 Windows 服务作为"路由真相源"，监听网络变化事件，自动重新下发路由；配合一个桌面 GUI 做配置、查看状态和诊断。

**产品名**：NetSwitcher（代码中包名、二进制名、安装目录名统一用此）。

---

## 2. 目标与非目标

### 目标
1. 用户通过 GUI 配置"哪些网段走哪块网卡"的规则，无需手敲命令。
2. 服务常驻、开机自启，监听网络变化，自动维持路由生效（解决 `route -p` 失效问题）。
3. 自动处理网关冲突与接口 metric（让默认路由走指定外网网卡）。
4. 提供 profile 切换（不同场景不同规则集）。
5. 提供路由表可视化与 tracert/ping 诊断。
6. 单二进制 + 一个 JSON 配置文件，部署简单。

### 非目标（第一版不做）
- 多用户/远程管理（仅本机）。
- 跨平台（仅 Windows 10/11 x64）。
- 与企业 VPN 客户端深度集成（仅做冲突检测与告警，不主动接管 VPN 路由）。
- 流量级负载均衡 / 双 WAN 聚合。
- IPv6（第一版仅 IPv4；接口预留 IPv6 字段但不处理）。
- 操作审计 / 合规日志归档（仅本地运行日志）。

---

## 3. 整体架构

### 3.1 组件关系

```
┌──────────────────────────────────────────────────────────┐
│  NetSwitcher GUI  (Wails v2 桌面应用, 用户会话, 非提权)     │
│  ┌──────────┬──────────┬──────────┬──────────┬────────┐  │
│  │  状态     │  Profile │ 路由表    │  诊断    │  日志   │  │
│  │  Status  │ Config   │ Routes   │ Diag     │ Logs   │  │
│  └──────────┴──────────┴──────────┴──────────┴────────┘  │
└──────────────────────┬───────────────────────────────────┘
                       │ IPC: 命名管道 JSON 协议
                       │ \\.\pipe\NetSwitcher
                       ▼
┌──────────────────────────────────────────────────────────┐
│  NetSwitcher Service  (Windows Service, 运行为 SYSTEM)     │
│                                                            │
│  ┌───────────────┐   ┌──────────────┐  ┌──────────────┐   │
│  │ Config Loader │──▶│ Route Engine │─▶│ State Store  │   │
│  │  (config.json)│   │  (apply/     │  │ (applied.json│   │
│  │  watch + reload│  │  reconcile)  │  │  上次下发)   │   │
│  └───────────────┘   └──────┬───────┘  └──────────────┘   │
│                             │                              │
│  ┌───────────────┐   ┌──────▼───────┐  ┌──────────────┐   │
│  │ Conflict      │◀──│ Network      │  │ IPC Server   │   │
│  │ Detector      │   │ Watcher      │  │ (named pipe) │   │
│  └───────────────┘   │ (poll 2s)    │  └──────────────┘   │
│                      └──────────────┘                      │
│  ┌───────────────┐   ┌──────────────┐  ┌──────────────┐   │
│  │ Interface     │   │ Logger       │  │ Metric Mgr   │   │
│  │ Manager       │   │ (zerolog →   │  │ (接口跃点数)  │   │
│  │ (name↔index)  │   │  file+pipe)  │  │              │   │
│  └───────────────┘   └──────────────┘  └──────────────┘   │
└──────────────────────────────────────────────────────────┘
                            │
                            ▼  执行命令
              route.exe / netsh.exe（subprocess）
```

### 3.2 单二进制双角色

**一个 `.exe`，通过子命令切换角色**（部署简单、版本统一）：

- `netswitcher.exe service install` — 安装为 Windows 服务（提权）
- `netswitcher.exe service uninstall` — 卸载（提权）
- `netswitcher.exe service start|stop` — 启停（提权）
- `netswitcher.exe service run` — 前台运行服务逻辑（**调试用**，无需安装）
- `netswitcher.exe gui` — 启动桌面 GUI
- `netswitcher.exe apply` — 读 config 应用一次后退出（**调试/CLI 验证用**）
- `netswitcher.exe dump` — 打印当前接口、配置、路由表（**调试用**）
- `netswitcher.exe --help`

GUI 主程序也是这个 exe；安装包在桌面/开始菜单放快捷方式指向 `netswitcher.exe gui`。

### 3.3 数据流
1. 用户在 GUI 改配置 → IPC → 服务写 `config.json` → 触发 apply。
2. 网络变化（Wi-Fi 重连等）→ Network Watcher 检测 → 触发 apply。
3. apply 时：读 config + 当前接口状态 → Route Engine 计算差异 → 删除失效路由 / 下发新路由 / 调整 metric → 写 State Store → 记日志 → 通过 IPC 推送状态给 GUI。

---

## 4. 技术栈与依赖

### 4.1 锁定的技术选型
- **语言**：Go 1.22+（用 `log/slog` 标准库做结构化日志）。
- **GUI**：Wails v2（`github.com/wailsapp/wails/v2`），前端用 **Svelte** 模板（编译型、体积小、agent 容易写）。WebView2 在 Win11 预装，Win10 安装包带 redist。
- **Windows 服务**：`github.com/kardianos/service`（事实标准）。
- **Windows API**：`golang.org/x/sys/windows`（接口枚举、命名管道 ACL 等）。
- **命名管道**：`github.com/Microsoft/go-winio`。
- **路由下发**：subprocess 调 `route.exe` / `netsh.exe`（可靠、零依赖、易调试；**不**直接 P/Invoke iphlpapi，避免维护 syscall wrapper）。
- **配置**：`encoding/json` 标准库 + 文件监听用 `github.com/fsnotify/fsnotify`。
- **日志**：`log/slog` → 同时输出到文件（`%ProgramData%\NetSwitcher\logs\`）和命名管道（供 GUI 实时查看）。
- **CLI 框架**：`github.com/spf13/cobra`。

### 4.2 完整依赖清单（go.mod）
```
github.com/wailsapp/wails/v2      v2.9.*      // 桌面 GUI 框架
github.com/kardianos/service      v1.2.*      // Windows 服务
github.com/Microsoft/go-winio     v0.6.*      // 命名管道
golang.org/x/sys                  latest      // Windows API
github.com/fsnotify/fsnotify      v1.7.*      // 配置文件监听
github.com/spf13/cobra            v1.8.*      // CLI
github.com/google/uuid            v1.6.*      // profile/rule ID
```
> agent 应使用各库最新的稳定 tag，不要锁死在示例的小版本号上；`go mod tidy` 后取最新 patch。

---

## 5. 项目结构

```
netswitcher/
├── go.mod
├── go.sum
├── wails.json                       // Wails 项目配置
├── README.md
├── build/                           // 构建产物（.ico, 安装脚本）
│   ├── windows/
│   │   ├── icon.ico
│   │   └── installer.nsi            // NSIS 安装脚本
├── cmd/
│   └── netswitcher/
│       └── main.go                  // 入口，cobra 路由子命令
├── internal/
│   ├── config/                      // 配置结构 + 加载/保存 + 监听
│   │   ├── config.go                // Config / Profile / Rule 结构
│   │   ├── load.go
│   │   ├── save.go
│   │   └── watch.go                 // fsnotify
│   ├── ifacemgr/                    // 接口管理
│   │   └── ifacemgr.go              // 枚举、按名称解析 index/gateway
│   ├── routeengine/                 // 路由引擎
│   │   ├── engine.go                // Apply / Reconcile
│   │   ├── exec.go                  // 封装 route.exe / netsh.exe
│   │   └── metric.go                // 接口 metric 设置
│   ├── netwatch/                    // 网络变化监听
│   │   └── watcher.go               // 轮询 net.Interfaces()，去抖
│   ├── conflict/                    // 冲突检测
│   │   └── detector.go              // 检测 VPN/其他来源的路由冲突
│   ├── state/                       // 上次下发状态持久化
│   │   └── state.go
│   ├── ipc/                         // IPC 协议
│   │   ├── protocol.go              // Request/Response 结构
│   │   ├── server.go                // 服务端（命名管道）
│   │   └── client.go                // 客户端（GUI 用）
│   ├── service/                     // Windows 服务包装
│   │   └── service.go               // kardianos/service 接入
│   ├── core/                        // 服务主循环，串联各模块
│   │   └── core.go
│   └── logging/
│       └── logging.go               // slog 配置 + 多路输出
├── pkg/                             // 可被 GUI 端复用的小工具
│   └── winutil/                     // 提权检测、程序目录等
├── frontend/                        // Wails 前端（Svelte）
│   ├── package.json
│   ├── vite.config.ts
│   ├── src/
│   │   ├── App.tsx                  // 等 — 见 8.3（实际用 .svelte）
│   │   ├── pages/
│   │   │   ├── Status.svelte
│   │   │   ├── Profiles.svelte
│   │   │   ├── Routes.svelte
│   │   │   ├── Diagnostics.svelte
│   │   │   └── Logs.svelte
│   │   ├── lib/
│   │   │   ├── ipc.ts               // 调用 Wails 后端绑定
│   │   │   └── types.ts             // TS 类型（与 Go 结构对齐）
│   │   └── components/
│   └── wailsjs/                     // Wails 自动生成的绑定
└── Makefile / build.ps1             // 构建脚本
```

> 注：上面 `frontend/src` 写法混了 React/Svelte 后缀仅为示意，**实际统一用 Svelte**（`.svelte` 文件）。

---

## 6. 配置文件设计

### 6.1 路径
- 配置：`%ProgramData%\NetSwitcher\config.json`
- 状态：`%ProgramData%\NetSwitcher\state.json`（服务写，记录上次成功下发的路由）
- 日志：`%ProgramData%\NetSwitcher\logs\netswitcher.log`（按天滚动，保留 7 天）
- IPC token：`%ProgramData%\NetSwitcher\runtime\ipc.lock`（可选）

`%ProgramData%` 默认对所有用户可读但仅 Admin 可写；服务以 SYSTEM 写入无障碍，GUI 通过 IPC 让服务代写配置，不直接写文件。

### 6.2 JSON Schema

```json
{
  "$schema": "https://example.invalid/netswitcher/config.v1.json",
  "version": 1,
  "activeProfile": "office",
  "profiles": [
    {
      "id": "office",
      "name": "办公区",
      "rules": [
        {
          "id": "r1",
          "destination": "168.168.0.0/16",
          "viaInterface": "以太网",
          "viaGateway": "auto",
          "metric": 1,
          "enabled": true
        },
        {
          "id": "r2",
          "destination": "172.16.0.0/16",
          "viaInterface": "以太网",
          "viaGateway": "auto",
          "metric": 1,
          "enabled": true
        }
      ],
      "defaultRouteInterface": "WLAN",
      "autoManageMetrics": true,
      "metricPolicy": {
        "preferredInterface": "WLAN",
        "preferredMetric": 10,
        "othersMetric": 50
      }
    }
  ],
  "logLevel": "info"
}
```

### 6.3 字段语义
| 字段 | 必填 | 说明 |
|---|---|---|
| `version` | 是 | schema 版本，当前固定 `1`，用于未来迁移 |
| `activeProfile` | 是 | 当前生效的 profile id |
| `profiles[].id` | 是 | 唯一，uuid 或 slug |
| `profiles[].name` | 是 | GUI 显示名 |
| `profiles[].rules[].destination` | 是 | CIDR，如 `168.168.0.0/16`；非法 CIDR 在保存时拒绝 |
| `profiles[].rules[].viaInterface` | 是 | 接口名（Windows 显示的"以太网"/"WLAN"，或 Description）。**匹配规则见 7.3** |
| `profiles[].rules[].viaGateway` | 是 | `"auto"` = 自动取该接口当前默认网关；或显式 IP |
| `profiles[].rules[].metric` | 否 | 路由 metric，默认 `1` |
| `profiles[].rules[].enabled` | 否 | 默认 `true`，禁用的规则下发时跳过 |
| `profiles[].defaultRouteInterface` | 否 | 默认路由应走哪个网卡；设了就触发 metric 管理 |
| `profiles[].autoManageMetrics` | 否 | 默认 `true`，自动调整接口跃点数让默认路由走对网卡 |
| `metricPolicy` | 否 | metric 策略；不填用默认值 |

### 6.4 配置校验规则（保存时严格执行）
1. CIDR 合法（用 `netip.ParsePrefix`）。
2. `viaInterface` 能在当前接口列表里解析到（解析失败给警告但允许保存，等接口出现再下发）。
3. `viaGateway` 是合法 IPv4，或字符串 `"auto"`。
4. 同一 profile 内不允许出现 `destination + viaInterface` 完全重复的规则。
5. `activeProfile` 必须指向已存在的 profile id。
6. 校验失败的写操作整体回滚，返回字段级错误信息给 GUI。

---

## 7. 服务端模块详解

### 7.1 核心循环（internal/core）
`core.Core` 持有所有子模块的引用，是服务的"心脏"。生命周期：

```
Start():
  1. loadConfig()           // 读 config.json，失败用空配置
  2. startLogger()
  3. ifaceMgr = New()       // 不阻塞，懒枚举
  4. state = Load()         // 读 state.json
  5. routeEngine = New(ifaceMgr, state)
  6. conflictDetector = New(ifaceMgr)
  7. ipcServer.Start()      // 监听命名管道
  8. netwatch.Start(cb=onNetworkChange)   // 启动轮询
  9. configWatch.Start(cb=onConfigChange) // 监听 config.json
 10. applyOnce(reason="startup")          // 启动时下发一次

onNetworkChange(change):
  debounce 1500ms → applyOnce(reason="network_change: " + change)

onConfigChange():
  reloadConfig() → applyOnce(reason="config_change")

applyOnce(reason):
  1. snapshot = ifaceMgr.Snapshot()
  2. conflicts = conflictDetector.Check(activeProfile, snapshot)
  3. result = routeEngine.Apply(activeProfile, snapshot)
  4. state.Save(result)
  5. log + emit IPC event "status.changed"

Stop():
  停 netwatch / configWatch / ipcServer；可选 cleanup 路由（保留，不删）
```

**关键决策**：服务**不**在停止时清理它下发的路由——避免关服务瞬间断网；路由是 runtime 的，重启系统自然清空，服务启动再下发。

### 7.2 配置加载器（internal/config）
- `Load() (*Config, error)`：读 `config.json`，做 schema 校验，返回结构。
- `Save(*Config) error`：原子写（写到 `.tmp` 再 `os.Rename`）。
- `Watch(cb func())`：用 fsnotify 监听 `config.json`；外部修改（包括 GUI 通过 IPC 让服务自己写）都会触发回调。**注意**：服务自己写配置时要抑制回调（用 `silentWrite` 标志位），避免循环。

### 7.3 接口管理器（internal/ifacemgr）
职责：把"接口名"解析成 `route.exe` 需要的 index 和当前 gateway。

```go
type Snapshot struct {
    Interfaces []Interface
    TakenAt    time.Time
}
type Interface struct {
    Index        int      // route.exe IF 参数用这个
    Name         string   // "以太网" / "WLAN"（NetConnectionID）
    FriendlyName string   // 适配器 Description
    MAC          string
    IPv4         []string // "172.16.5.10/24"
    Gateways     []string // 当前 IPv4 默认网关，["172.16.5.1"]
    IsUp         bool
    MediaType    string   // "WiFi" / "Ethernet" / ...
}
```

实现：用 `net.Interfaces()` + `net.Addrs()` 拿到 IP；用 Windows API `GetAdaptersAddresses`（通过 `golang.org/x/sys/windows` 调）拿网关、Name、Description、MediaType。如果 `golang.org/x/sys/windows` 暴露不足，可走 `Get-NetAdapter | ConvertTo-Json`（PowerShell，仅在 GUI/dump 调用，不在热路径）。

**接口名匹配规则**（按优先级，命中即停）：
1. 完全匹配 `Interface.Name`
2. 完全匹配 `Interface.FriendlyName`（Description）
3. 不区分大小写包含匹配
4. 都不命中 → 返回 `ErrInterfaceNotFound`，路由引擎跳过依赖该接口的规则并记 warning

### 7.4 路由引擎（internal/routeengine）
核心模块。`Apply` 做的是"声明式 reconcile"：让系统路由表符合配置，而不是简单 add/delete。

```go
type ApplyResult struct {
    Applied []RouteEntry    // 本次成功下发的
    Removed []RouteEntry    // 本次删除的
    Skipped []SkippedRule   // 因接口缺失等跳过的规则
    Errors  []RuleError     // 下发失败的
    At      time.Time
}
type RouteEntry struct {
    Destination string  // "168.168.0.0/16"
    Gateway     string
    Interface   string
    IfIndex     int
    Metric      int
}
```

**Apply 流程**：
```
1. 读 state.json 拿"上次下发的路由集合" oldSet
2. 根据 activeProfile 计算出"应该存在的路由集合" wantSet
   - 每条 enabled rule：
     - 解析 viaInterface → IfIndex（失败记 Skipped，跳过）
     - 解析 viaGateway：auto → 取 ifaceMgr.Snapshot 中该接口第一个 Gateway
   - 构造 RouteEntry
3. diff = wantSet - oldSet（要新增）；obsolete = oldSet - wantSet（要删除）
4. 删除 obsolete：对每条调 `route delete <dest> IF <ifIndex>`
   - 注意：仅删我们之前下发的（state.json 记录的），不碰系统/VPN 路由
5. 新增 diff：对每条调 `route add <dest> mask <mask> <gateway> IF <ifIndex> metric <m>`
   - **不带** `-p`（runtime 路由，重启清空，服务再下发）
6. 若 profile.autoManageMetrics：调 metric 管理器
7. 把 wantSet 写回 state.json
8. 返回 ApplyResult
```

**幂等性**：apply 多次结果一致。重复 add 同一条 `route.exe` 会报错"对象已存在"，exec.go 要把这个错误识别为成功（idempotent）。

**CIDR 拆分**：`168.168.0.0/16` → destination=`168.168.0.0` mask=`255.255.0.0`。用 `netip.ParsePrefix` 解析后格式化。

### 7.5 metric 管理（internal/routeengine/metric.go）
```
策略：让 preferredInterface 抢到默认路由 0.0.0.0/0
preferredInterface 解析优先级：
  1. metricPolicy.preferredInterface（若显式配置）
  2. 否则用 profile.defaultRouteInterface
  若两者都未配置 → 不做 metric 管理，记 warning
实现：
  netsh interface ipv4 set interface "<preferred>" metric=<preferredMetric>
  netsh interface ipv4 set interface "<other1>"    metric=<othersMetric>
  ... 对所有 up 的 IPv4 接口
默认值：preferredMetric=10, othersMetric=50（配置未给 metricPolicy 时用）
注意：
  - metric 调整也要记进 state.json，stop 时不还原（重启自然还原为 DHCP/自动）
  - 不要碰接口的"自动跃点数"开关之外的东西
```

### 7.6 网络监听（internal/netwatch）
**用轮询，不用 NotifyAddrChange**（实现简单、可靠、跨版本稳定）：

```go
// 伪代码
func (w *Watcher) loop() {
    prev := snapshot()
    ticker := time.NewTicker(2 * time.Second)
    for {
        select {
        case <-ticker.C:
            cur := snapshot()
            if diff(prev, cur) {
                w.cb(describeDiff(prev, cur))
            }
            prev = cur
        case <-w.stop:
            return
        }
    }
}
```

**diff 判定**（任一成立即认为变化）：
- 任一接口 up/down 翻转
- 任一接口 IPv4 地址变化
- 任一接口 gateway 变化

**去抖**：cb 在 core 里被包一层 1500ms debounce，避免 Wi-Fi 重连过程中抖动多次 apply。

snapshot 复用 `ifaceMgr.Snapshot()`，不重复造轮子。

### 7.7 冲突检测（internal/conflict）
**目的**：发现"其他程序（VPN/虚拟网卡）也在管路由表"的情况，告警但不擅自覆盖。

```
Check(profile, snapshot) []Conflict:
  1. 读当前真实路由表（route print 解析，或 Get-NetRoute）
  2. 对每条 wantSet 中的 destination：
     - 如果当前已存在相同 destination 但下一跳/接口不一致，且不是 state.json 里的 → Conflict{type="external_override"}
  3. 检测 VPN 适配器（MediaType 含 "VPN" 或 FriendlyName 含 "WireGuard"/"OpenVPN"/"Cisco AnyConnect" 等关键字）：
     - 若存在且有默认路由，且 profile 想抢默认路由 → Conflict{type="vpn_present"}
  返回 Conflict 列表，附在 ApplyResult 里给 GUI。
```

不自动解决冲突——只报告。后续可加"忽略/强制覆盖"开关。

### 7.8 状态存储（internal/state）
`state.json` 记录上次成功 apply 的结果，结构同 `ApplyResult` 的精简版。用途：
- 下次 apply 时算 diff（只删自己下发的，不误删系统路由）
- GUI 启动时展示"当前实际下发状态"

### 7.9 IPC 服务端（internal/ipc/server.go）
见第 9 章。

### 7.10 日志（internal/logging）
- 用 `log/slog`，JSON handler。
- 输出：文件（按天 rotate，保留 7 天，单文件 >50MB 也 rotate）+ 命名管道（GUI 实时读）。
- 级别：debug/info/warn/error，配置文件可调。
- 关键事件必记：apply 开始/结束、每条 route add/delete 结果、网络变化、配置变化、IPC 请求、冲突。

---

## 8. GUI 模块（Wails v2 + Svelte）

### 8.1 页面结构（左侧导航 + 主区域）
1. **状态 Status** — 默认页
   - 顶部：当前活动 profile 名 + "立即重新应用"按钮
   - 接口卡片：每个网卡一张卡，显示 Name / IP / Gateway / up-down / MediaType
   - 当前下发路由摘要表（destination → interface → gateway → metric）
   - 最近一次 apply 时间、结果统计（成功 N 条 / 跳过 N 条 / 失败 N 条）
   - 冲突告警条（若有）
2. **配置 Profiles**
   - 左：profile 列表（高亮 active），"+ 新建"
   - 右：选中 profile 的编辑表单——规则表（行编辑：destination / viaInterface 下拉 / viaGateway / metric / enabled）、defaultRouteInterface 下拉、metricPolicy
   - 底部：保存 / 另存为 / 删除 / 设为活动
3. **路由表 Routes**
   - 解析后的当前系统路由表，可排序、搜索
   - 每行标记来源：[本工具下发] / [系统] / [疑似 VPN]
   - 行右键：tracert / ping
4. **诊断 Diagnostics**
   - 输入框：目标 IP/域名
   - 按钮：ping / tracert
   - 输出区：文本流式输出（IPC 走 streaming，见 9.4）
   - 显示"实际走的接口"
5. **日志 Logs**
   - 滚动列表，级别筛选，关键词过滤
   - 实时推送（IPC 订阅）

### 8.2 Wails 后端绑定（Go 侧）
GUI 进程不直接调 route.exe，**只通过 IPC 客户端跟服务通信**。Wails 暴露给前端的方法（`frontend/wailsjs/` 自动生成 TS）：

```go
type API struct{}
func (a *API) GetStatus() (StatusResponse, error)
func (a *API) GetConfig() (Config, error)
func (a *API) SaveProfile(p Profile) error          // 内部：GetConfig→改→SaveConfig
func (a *API) DeleteProfile(id string) error        // 内部：GetConfig→删→SaveConfig
func (a *API) SetActiveProfile(id string) error     // 直通 IPC SetActiveProfile
func (a *API) ApplyNow() (ApplyResult, error)
func (a *API) GetRouteTable() ([]RouteRow, error)
func (a *API) Ping(target string) (<-chan string, error)   // 流式
func (a *API) Tracert(target string) (<-chan string, error)
func (a *API) GetLogs(filter LogFilter) (<-chan LogEntry, error)  // 流式订阅（对应 IPC SubscribeLogs）
```

**层关系（重要）**：IPC（第 9 章）只暴露**整配置**粒度的方法（`GetConfig` / `SaveConfig`）。Wails 层的 `SaveProfile` / `DeleteProfile` 是 GUI 侧的便利方法，实现是 `GetConfig → 内存里改 profile → SaveConfig` 三步组合，不是独立 IPC 方法。这样保持 IPC 简单，又让前端能按 profile 粒度操作。流式方法（`Ping`/`Tracert`/`GetLogs`）一对一映射到 IPC 的 `Ping`/`Tracert`/`SubscribeLogs`。

**服务未运行时**：所有 IPC 调用失败 → GUI 显示横幅"服务未运行"，提供"以管理员身份启动服务"按钮（提权拉起 `netswitcher.exe service start`，用 `cmd /c start /b` + ShellExecute runas）。

### 8.3 前端约定
- Svelte 组件放 `frontend/src/pages/` 和 `frontend/src/components/`。
- 调后端：`import { GetStatus, ApplyNow, ... } from '../wailsjs/go/main/API'`。
- 类型：`frontend/src/lib/types.ts` 与 Go 结构手动对齐（或用 `wails generate` 同步）。
- 状态管理：Svelte store 即可，不引第三方。
- 样式：原生 CSS 或内联 style；避免引大型 UI 库（保持体积）；可用 `picocss` 一类极轻量 css。

---

## 9. IPC 协议

### 9.1 传输
- 命名管道：`\\.\pipe\NetSwitcher`
- 编码：行分隔 JSON（每行一个完整 JSON 对象）。请求一行、响应一行（或流式多行）。
- ACL：仅本地 SYSTEM + 本地登录用户（`D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;IU)`）。无需 token。

### 9.2 消息格式
```json
// Request
{"id": "req-1", "method": "ApplyNow", "params": {}}

// Response（成功）
{"id": "req-1", "ok": true, "result": { ... }}

// Response（失败）
{"id": "req-1", "ok": false, "error": {"code": "IFACE_NOT_FOUND", "message": "..."}}

// 流式（ping/tracert/logs）
{"id": "req-1", "stream": true, "data": "..."}      // 多条
{"id": "req-1", "stream": "end"}                     // 结束标记
```

### 9.3 方法清单
| method | params | result | 说明 |
|---|---|---|---|
| `GetStatus` | `{}` | `StatusResponse` | 接口快照 + 活动profile + 上次apply结果 + 冲突 |
| `GetConfig` | `{}` | `Config` | 完整配置 |
| `SaveConfig` | `{config: Config}` | `{}` | 整体替换（带校验） |
| `SetActiveProfile` | `{id: string}` | `{}` | 切换并触发 apply |
| `ApplyNow` | `{}` | `ApplyResult` | 强制重新应用 |
| `GetRouteTable` | `{}` | `[]RouteRow` | 当前真实路由表 |
| `Ping` | `{target: string}` | stream `string` | 流式 ping 输出 |
| `Tracert` | `{target: string}` | stream `string` | 流式 tracert 输出 |
| `SubscribeLogs` | `{level?: string}` | stream `LogEntry` | 订阅日志推送 |
| `SubscribeStatus` | `{}` | stream `StatusResponse` | 状态变化推送（apply 完成等） |

错误码（`error.code`）：`IFACE_NOT_FOUND` / `INVALID_CIDR` / `INVALID_CONFIG` / `ROUTE_EXEC_FAILED` / `INTERNAL` / `SERVICE_BUSY`。

### 9.4 流式实现
- 服务端为每个流式请求开一个 goroutine，把输出通过 channel 发回。
- 同一管道连接可并发多请求（用 id 区分）；但为简单起见，**第一版每条流式请求单独开一个管道连接**。
- 客户端读循环按 `id` 分发到对应 channel。

---

## 10. 关键流程伪代码

### 10.1 服务启动
```
main → cobra 命中 "service run" 或被 SCM 拉起
  → kardianos/service 接入：如果是 service 模式，注册 SCM handler
  → svc.Start() 里：
       core := NewCore(configPath, statePath)
       core.Start()   // 阻塞直到收到 stop
```

### 10.2 应用配置（applyOnce）
```
func (c *Core) applyOnce(reason string) ApplyResult {
    snapshot := c.ifaceMgr.Snapshot()
    profile := c.config.ActiveProfile()

    want := computeWantSet(profile, snapshot)         // 7.4
    old := c.state.LastApplied()

    toAdd, toRemove := diff(want, old)
    for r := range toRemove { c.routeExec.Delete(r) }  // 仅删自己的
    for r := range toAdd   { c.routeExec.Add(r) }

    if profile.AutoManageMetrics {
        c.metricMgr.Apply(profile, snapshot)
    }

    conflicts := c.conflict.Check(want, snapshot)
    result := buildResult(want, toAdd, toRemove, conflicts)
    c.state.Save(result)
    c.ipc.Emit("status.changed", result)              // 推 GUI
    c.log.Info("apply done", "reason", reason, "added", len(toAdd), ...)
    return result
}
```

### 10.3 网络变化处理
```
netwatch.cb(change) →
  debounce 1500ms →
    c.applyOnce("network_change: " + change)
```

### 10.4 配置变化（GUI 改了配置）
```
GUI SaveProfile → IPC SaveConfig → 服务 config.Save(silent=true)
  → fsnotify 触发（silent 标志抑制）→ core.onConfigChange
  → reloadConfig + applyOnce("config_change")
  → 返回 ApplyResult 给 GUI
```

### 10.5 GUI 启动
```
netswitcher.exe gui →
  Wails app 启动 →
  前端 mounted →
  调 GetStatus / GetConfig →
  若 IPC 连接失败 → 显示"服务未运行"横幅 + 启动按钮
  否则渲染状态页
```

---

## 11. Windows 命令与 API 参考

agent 在 exec.go 里直接 subprocess 调用以下命令。所有命令以 SYSTEM 身份运行（服务进程），不需要再提权。

### 11.1 路由下发
```
# 新增（runtime，不持久化）
route add <dest> mask <mask> <gateway> IF <ifIndex> metric <m>

# 删除（按 destination 删；IF 可选，多接口同 dest 时必带）
route delete <dest>

# 查看
route print -4
```

**注意**：
- 命令的 stderr/stdout 都要捕获；`route add` 重复添加会输出"对象已存在"→ 识别为成功。
- 中文 Windows 下输出可能是 GBK，解码用 `golang.org/x/text/encoding/simplifiedchinese.GBK`。

### 11.2 接口 metric
```
netsh interface ipv4 set interface name="<name>" metric=<n>
# 恢复自动：
netsh interface ipv4 set interface name="<name>" metric=automatic
```

### 11.3 路由表读取（用于 Routes 页和冲突检测）
优先用 PowerShell 拿结构化数据：
```
powershell -NoProfile -Command "Get-NetRoute -AddressFamily IPv4 | Select-Object DestinationPrefix,NextHop,InterfaceIndex,RouteMetric,InterfaceMetric | ConvertTo-Json"
```
PowerShell 启动慢（~500ms），**仅用于 GUI 触发的查看**，不放进 apply 热路径（apply 用 state.json 做 diff，不读 route table）。

### 11.4 接口枚举
- 主路径：Go `net.Interfaces()` + `golang.org/x/sys/windows` 调 `GetAdaptersAddresses`。
- 兜底：`powershell Get-NetAdapter | ConvertTo-Json`。

### 11.5 ping / tracert
直接 subprocess 系统 `ping.exe` / `tracert.exe`，行流式回传给 GUI。

---

## 12. 实现阶段（核心，agent 按序执行）

每个 phase 都要：① 完成列出的任务 ② 通过验收标准 ③ 提交一次（commit）后再开始下一个 phase。**严格按顺序**，后阶段依赖前阶段。

### Phase 0 — 项目骨架与构建（预计 0.5 天）
**目标**：跑通"单二进制 + 子命令 + Wails 空壳"。

任务：
1. `wails init -n netswitcher -t svelte`，把项目改造为上面的目录结构。
2. 加 cobra：`cmd/netswitcher/main.go` 路由 `service install/uninstall/start/stop/run`、`gui`、`apply`、`dump`、`--help`。
3. `service run` / `apply` / `dump` 先打一行 log，`gui` 调 `wails.Run`。
4. Makefile / build.ps1：`make build` 产出 `netswitcher.exe`。
5. README 写最小启动说明。

**验收**：
- `make build` 成功，生成单 exe。
- `netswitcher.exe --help` 列出所有子命令。
- `netswitcher.exe service run` 前台跑起来不崩、Ctrl-C 退出干净。
- `netswitcher.exe gui` 打开 Wails 窗口。

---

### Phase 1 — 配置 + 接口枚举（纯逻辑，预计 1 天）
**目标**：能加载/校验/保存配置，能枚举本机接口。**这一阶段不碰路由表**。

任务：
1. 实现 `internal/config`：结构、Load、Save（原子写）、Validate（6.4 所有规则）、Watch（fsnotify + silent 抑制）。
2. 实现 `internal/ifacemgr`：Snapshot 返回 `[]Interface`（含 index/name/IP/gateway/MediaType）；ResolveByName 实现 7.3 匹配规则。
3. 实现 `cmd dump`：打印当前接口列表 + 加载的 config（pretty JSON）。
4. 写单测：config 校验（合法/非法 CIDR、重复规则、未知 profile）、接口名匹配。

**验收**：
- `netswitcher.exe dump` 输出本机所有 IPv4 接口（与 `ipconfig` 一致），含正确的 gateway。
- 故意写错 CIDR 的 config.json，dump 报清晰错误。
- `go test ./internal/config ./internal/ifacemgr` 全绿。

---

### Phase 2 — 路由引擎（核心，预计 1.5 天）
**目标**：`netswitcher.exe apply` 能根据 config 把路由下发到本机，且幂等。

任务：
1. 实现 `internal/routeengine/exec.go`：`Add(RouteEntry)` / `Delete(RouteEntry)`，封装 route.exe，处理 GBK 解码、"对象已存在"识别。
2. 实现 `internal/routeengine/engine.go`：computeWantSet、diff、Apply（按 7.4 流程）。
3. 实现 `internal/state`：Save/Load ApplyResult。
4. 实现 metric 管理（7.5）。
5. 实现 `cmd apply`：load config → snapshot → engine.Apply → 打印 result → exit。
6. 手工测试：在 config 里放一条规则，apply，`route print` 看到正确路由；再 apply 一次（幂等）；改 config 再 apply（diff 正确增删）。

**验收**：
- 在测试机（双网卡）apply 后，`route print` 出现配置要求的所有路由，下一跳和 IF 正确。
- 重复 apply 不报错、不重复添加。
- 改一条规则再 apply，旧路由被删、新路由被加。
- 默认路由确实走 `defaultRouteInterface`（`tracert 8.8.8.8` 第一跳是 Wi-Fi 网关）。
- 故意配一个不存在的接口名，apply 不崩，返回 Skipped。

> **本阶段在真实双网卡环境验证**——agent 应在 VM 里配两块网卡（一块 host-only/内部、一块 NAT）测试。

---

### Phase 3 — 网络监听 + 自动应用（预计 1 天）
**目标**：服务前台模式（`service run`）能监听网络变化自动重下发。

任务：
1. 实现 `internal/netwatch`：2s 轮询、diff 判定、回调（7.6）。
2. 实现 `internal/core`：Start/Stop、applyOnce、debounce 包装 onNetworkChange、onConfigChange。
3. `service run` 启动 core；SIGINT/SCM stop 优雅退出。
4. 测试场景：apply 后禁用再启用 Wi-Fi → 等待 ≤4s → 路由自动恢复；插拔网线同理；改 config.json → 自动 reload + apply。

**验收**：
- Wi-Fi 重连后，`route print` 中目标路由在 ≤5s 内重新出现。
- 日志能看到 `reason="network_change: ..."` 的 apply 记录。
- 配置文件外部修改后 2s 内触发 apply。

---

### Phase 4 — 服务化（kardianos/service，预计 0.5 天）
**目标**：能作为 Windows 服务安装、开机自启、重启后自动工作。

任务：
1. 实现 `internal/service/service.go`：用 kardianos/service 包装 core；install/uninstall/start/stop 子命令；服务显示名 "NetSwitcher"，描述 "内外网路由管理"，启动类型自动。
2. 服务以 SYSTEM 身份运行；`%ProgramData%\NetSwitcher\` 目录在 install 时创建并设 ACL。
3. 日志路径在服务模式下指向 `%ProgramData%\NetSwitcher\logs\`。
4. 测试：install → services.msc 看到 → start → 重启系统 → 登录前路由已生效。

**验收**：
- `netswitcher.exe service install`（管理员）成功，services.msc 出现服务。
- 服务状态 Running；`route print` 路由生效。
- 重启系统后（不登录）路由仍在（服务 SCM 自动启动）。
- `service uninstall` 干净卸载。

---

### Phase 5 — IPC（命名管道，预计 1 天）
**目标**：服务暴露所有方法，CLI 测试客户端能完整调用。

任务：
1. 实现 `internal/ipc/protocol.go`：Request/Response/Stream 消息结构。
2. 实现 `server.go`：命名管道监听（go-winio）、JSON 行协议、ACL、并发处理、所有 9.3 方法。
3. 实现 `client.go`：GUI 用的客户端，支持单请求和流式订阅。
4. 实现 `core` 的事件 emit（`status.changed`）→ IPC 推给订阅者。
5. 加一个隐藏子命令 `netswitcher.exe ipc call <method> <json>` 用于 CLI 自测。

**验收**：
- 服务运行时，`netswitcher.exe ipc call GetStatus {}` 返回正确的 JSON。
- `SetActiveProfile` 后 `GetStatus` 反映新 profile。
- 流式方法（Ping）能逐行收到输出，最后收到 `stream:end`。
- 用 `pipelist.exe` 或自测客户端验证 ACL：非管理员本机用户能连，远程不能（管道名仅本地）。

---

### Phase 6 — GUI（Wails + Svelte，预计 2 天）
**目标**：完整可用的桌面应用，覆盖 8.1 全部 5 个页面。

任务：
1. Wails 后端 `API` 结构（8.2），全部走 IPC client。
2. 前端 5 个页面组件 + 路由 + 导航。
3. 状态页：接口卡片、下发路由表、apply 按钮、冲突横幅。
4. 配置页：profile 列表 + 规则表编辑 + 校验错误展示（字段级）。
5. 路由表页：调 GetRouteTable，标记来源，搜索排序。
6. 诊断页：ping/tracert 流式输出。
7. 日志页：订阅推送 + 级别过滤。
8. 服务未运行检测 + 提权启动服务按钮。
9. 系统托盘（可选，用 `fyne.io/systray` 或等价库）：最小化到托盘、状态指示。

**验收**：
- 启动 GUI 能正确显示当前接口和下发状态。
- 在配置页改一条规则并保存，3s 内状态页反映变化、`route print` 也变。
- ping/tracert 输出实时滚动。
- 日志页实时刷新。
- 故意停掉服务，GUI 显示横幅且按钮能拉起服务。

---

### Phase 7 — 打包与收尾（预计 1 天）
**目标**：可分发的安装包 + 文档。

任务：
1. NSIS 安装脚本（`build/windows/installer.nsi`）：
   - 装 exe 到 `%ProgramFiles%\NetSwitcher\`
   - 创建开始菜单/桌面快捷方式（指向 `gui`）
   - 安装时执行 `service install`（提权）
   - 卸载时 `service uninstall` + 询问是否删配置
   - 内置 WebView2 EverBootstrapper（Win10 兜底）
2. 图标、版本信息、签名（如有证书；无则跳过但文档说明）。
3. README：安装、卸载、常见问题（含"路由没生效怎么排查"）。
4. 用户手册（简短）：如何配第一条规则、如何切 profile、如何诊断。

**验收**：
- 在干净 Win10 / Win11 VM 上双击安装包，一路下一步完成。
- 安装后服务 Running，重启后仍 Running。
- 开始菜单能打开 GUI，全部功能可用。
- 卸载干净（服务移除、文件清理、可选保留配置）。

---

## 13. 测试策略

### 13.1 单元测试（必须）
- `internal/config`：校验规则全覆盖（合法/非法 CIDR、重复、未知 profile、原子写、silent 抑制）。
- `internal/ifacemgr`：接口名匹配优先级、未命中返回错误。
- `internal/routeengine`：diff 计算正确性（用 mock 的 exec，不真调 route.exe）。
- `internal/ipc`：协议编解码、错误码、流式边界。

### 13.2 集成测试（VM 双网卡）
- VM 配两块网卡：一块"内部网络"（172.16.0.0/16 段，无网关或网关不通外网），一块 NAT（外网）。
- 测试矩阵：
  1. apply 后路由表正确。
  2. Wi-Fi（NAT 网卡）断连重连，路由自动恢复。
  3. 配置变更自动应用。
  4. profile 切换。
  5. 服务重启、系统重启后路由生效。
  6. 不存在的接口名优雅跳过。
  7. VPN 适配器存在时冲突告警。

### 13.3 手工冒烟
每次 phase 完成跑一遍对应章节的"验收"清单。

---

## 14. 风险与边界情况

| 风险 | 缓解 |
|---|---|
| 中文 Windows 下 `route.exe` 输出 GBK | exec.go 用 `simplifiedchinese.GBK.NewDecoder()`；加单测 |
| route.exe "对象已存在" 错误 | 识别为幂等成功，不算失败 |
| 网卡 IF index 在重连后变化 | 路由不持久化（不带 -p），每次 apply 用当前 snapshot 重新解析 index |
| DHCP 还没拿到 IP 时 apply | viaGateway=auto 解析不到 → Skipped，等下一次网络变化重试 |
| 与 VPN 客户端冲突 | 冲突检测器告警，不主动覆盖；后续可加"忽略 VPN"开关 |
| 服务崩溃 | kardianos 配置恢复策略（RestartOnFailure）；state.json 损坏用空状态启动 |
| GUI 在没有服务的机器上跑 | 检测 IPC 失败 → 横幅 + 提权启动按钮 |
| 用户多 profile 切换频繁 | debounce 避免抖动；SetActiveProfile 是同步 IPC，前端 disable 按钮防双击 |
| WebView2 不存在（旧 Win10） | 安装包带 EverBootstrapper；首次启动检测 |
| PowerShell 执行策略限制 | 用 `-NoProfile -ExecutionPolicy Bypass` 调用 |
| 配置文件被外部改坏 | Load 失败时不覆盖，记 error，用上次成功配置（缓存）继续 |

---

## 15. 总验收标准（全部 phase 完成后）

1. **功能**：在一台真实双网卡 Win11 上，配置 `168.168.0.0/16` 和 `172.16.0.0/16` 走以太网、其余走 Wi-Fi，apply 后内网服务器/终端可达、外网正常（tracert 验证）。
2. **稳定性**：Wi-Fi 断连重连、插拔网线、重启系统后，路由 ≤5s 内自动正确恢复，无需人工干预。
3. **可用性**：非技术用户凭 README 和 GUI 能完成首次配置。
4. **部署**：单安装包，干净 Win10/Win11 可一键安装/卸载。
5. **可观测**：日志完整，能从日志还原"何时因为什么 apply 了什么"。
6. **测试**：单测全绿；VM 集成测试矩阵全部通过。

---

## 16. 附录：关键代码骨架

> 这些是给 agent 的起手示例，**不是最终实现**。agent 应据此风格实现完整逻辑。

### 16.1 路由下发（internal/routeengine/exec.go）
```go
package routeengine

import (
    "bytes"
    "fmt"
    "os/exec"
    "strings"
    "unicode/utf8"

    "golang.org/x/text/encoding/simplifiedchinese"
)

type RouteEntry struct {
    Dest     string // "168.168.0.0"
    Mask     string // "255.255.0.0"
    Gateway  string
    IfIndex  int
    Metric   int
}

// decodeGBK 处理中文 Windows 的 route.exe 输出
func decode(b []byte) string {
    if utf8.Valid(b) {
        return string(b)
    }
    s, err := simplifiedchinese.GBK.NewDecoder().Bytes(b)
    if err != nil {
        return string(b)
    }
    return string(s)
}

func (e *Exec) Add(r RouteEntry) error {
    cmd := exec.Command("route", "add", r.Dest,
        "mask", r.Mask, r.Gateway,
        "IF", fmt.Sprint(r.IfIndex),
        "metric", fmt.Sprint(r.Metric))
    var out, errB bytes.Buffer
    cmd.Stdout = &out
    cmd.Stderr = &errB
    err := cmd.Run()
    msg := decode(out.Bytes()) + decode(errB.Bytes())
    // 幂等：已存在视为成功
    if err != nil && (strings.Contains(msg, "已存在") || strings.Contains(msg, "exists")) {
        return nil
    }
    if err != nil {
        return fmt.Errorf("route add failed: %s: %w", msg, err)
    }
    return nil
}

func (e *Exec) Delete(dest string, ifIndex int) error {
    args := []string{"delete", dest}
    if ifIndex > 0 {
        args = append(args, "IF", fmt.Sprint(ifIndex))
    }
    cmd := exec.Command("route", args...)
    out, err := cmd.CombinedOutput()
    msg := decode(out)
    // 不存在的路由删除不算错
    if err != nil && (strings.Contains(msg, "找不到") || strings.Contains(msg, "could not find")) {
        return nil
    }
    return err
}
```

### 16.2 网络监听去抖（internal/core）
```go
func (c *Core) onNetworkChange(desc string) {
    c.debouncer(func() {
        c.applyOnce("network_change: " + desc)
    })
}

// debounce：1500ms 内多次触发只执行最后一次
type Debouncer struct {
    mu    sync.Mutex
    timer *time.Timer
    d     time.Duration
    f     func()
}

func NewDebouncer(d time.Duration) *Debouncer { return &Debouncer{d: d} }

func (db *Debouncer) Call(f func()) {
    db.mu.Lock()
    defer db.mu.Unlock()
    if db.timer != nil {
        db.timer.Stop()
    }
    db.f = f
    db.timer = time.AfterFunc(db.d, func() {
        db.mu.Lock()
        f := db.f
        db.f = nil
        db.timer = nil
        db.mu.Unlock()
        if f != nil {
            f()
        }
    })
}
```

### 16.3 IPC handler 例子（internal/ipc/server.go）
```go
func (s *Server) handle(conn net.Conn) {
    defer conn.Close()
    scanner := bufio.NewScanner(conn)
    scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
    for scanner.Scan() {
        line := scanner.Bytes()
        var req Request
        if err := json.Unmarshal(line, &req); err != nil {
            s.write(conn, Response{ID: "", OK: false, Error: ec("INVALID", err.Error())})
            continue
        }
        resp := s.dispatch(req)
        s.write(conn, resp)
    }
}

func (s *Server) dispatch(req Request) Response {
    switch req.Method {
    case "GetStatus":
        st := s.core.Status()
        return Response{ID: req.ID, OK: true, Result: st}
    case "SetActiveProfile":
        var p struct{ ID string `json:"id"` }
        if err := json.Unmarshal(req.Params, &p); err != nil {
            return Response{ID: req.ID, OK: false, Error: ec("INVALID", err.Error())}
        }
        if err := s.core.SetActiveProfile(p.ID); err != nil {
            return Response{ID: req.ID, OK: false, Error: ec("INTERNAL", err.Error())}
        }
        return Response{ID: req.ID, OK: true, Result: s.core.Status()}
    // ... 其余方法
    }
    return Response{ID: req.ID, OK: false, Error: ec("UNKNOWN_METHOD", req.Method)}
}
```

### 16.4 命名管道 + ACL（internal/ipc/server.go）
```go
import "github.com/Microsoft/go-winio"

func (s *Server) listen() error {
    // SD: 允许 SYSTEM、Administrators、本地交互用户
    sd := "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;IU)"
    cfg := &winio.PipeConfig{
        SecurityDescriptor: sd,
        MessageMode:        false, // 字节流 + 行分隔 JSON
        InputBufferSize:    64 * 1024,
        OutputBufferSize:   64 * 1024,
    }
    ln, err := winio.ListenPipe(`\\.\pipe\NetSwitcher`, cfg)
    if err != nil {
        return err
    }
    s.listener = ln
    go s.acceptLoop()
    return nil
}
```

### 16.5 Wails 入口（cmd/netswitcher/main.go 片段）
```go
var guiCmd = &cobra.Command{
    Use:   "gui",
    Short: "启动桌面 GUI",
    Run: func(cmd *cobra.Command, args []string) {
        api := NewAPI() // 内部持 ipc.Client
        err := wails.Run(&options.App{
            Title:  "NetSwitcher",
            Width:  1024, Height: 700,
            AssetServer: &assetserver.Options{Assets: assets},
            OnStartup:   api.onStartup,
            Bind:        []interface{}{api},
        })
        if err != nil {
            log.Fatal(err)
        }
    },
}
```

### 16.6 前端调用绑定（frontend/src/lib/ipc.ts）
```typescript
import { GetStatus, ApplyNow, SaveProfile, SetActiveProfile, Ping, Tracert, GetRouteTable, GetLogs } from '../../wailsjs/go/main/API';
import type { StatusResponse, Profile, RouteRow, LogEntry } from './types';

export const ipc = {
  getStatus: () => GetStatus() as Promise<StatusResponse>,
  applyNow: () => ApplyNow(),
  saveProfile: (p: Profile) => SaveProfile(p),
  setActive: (id: string) => SetActiveProfile(id),
  routeTable: () => GetRouteTable() as Promise<RouteRow[]>,
  ping: (t: string) => Ping(t),
  tracert: (t: string) => Tracert(t),
};
```

---

## 17. 给 agent 的执行约束

1. **严格按 phase 顺序**，每 phase 提交后再开始下一个；不要跨 phase 提前实现。
2. **不擅自换技术栈**（Go / Wails v2 / Svelte / kardianos / go-winio 已锁）。
3. **不持久化路由**（绝不带 `-p`），全靠服务 runtime 下发 + 网络监听重下发。
4. **不删非自己下发的路由**——所有删除操作必须能在 state.json 里找到对应记录。
5. **遇到中文 Windows 兼容性**（GBK、PowerShell 执行策略）按"风险"章节处理，不要绕过。
6. **测试必须用 VM 双网卡环境**，单网卡机器测不出关键场景。
7. **每个 phase 完成跑一遍对应"验收"清单**，全部通过再 commit。
8. **日志先行**：任何模块第一个实现的就是日志，便于排查。
9. 出现本方案没覆盖的设计点，按"最小惊讶原则"决策并在 commit message 说明；不要卡住等人工。
10. 交付物：可构建的源码 + 单测 + 安装包 + README + 用户手册。

---

**方案结束。** 按 Phase 0 → 7 顺序执行即可交付。
