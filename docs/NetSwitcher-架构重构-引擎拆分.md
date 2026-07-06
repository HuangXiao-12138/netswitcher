# NetSwitcher 架构重构方案:GUI 与路由引擎拆分(D1)

> 状态:方案已定,待实施
> 决策日期:2026-07-06
> 方案代号:**D1**(引擎作为 GUI 的提权子进程)

---

## 一、背景与问题

### 1.1 当前架构

单进程:`netswitcher.exe` 同时承载 Wails GUI 和路由引擎(`internal/core`),GUI 进程必须**提权**(route.exe / netsh 需要管理员)。

```
netswitcher.exe (提权)
├── Wails GUI
├── core 路由引擎(apply / netwatch / route.exe / netsh)
├── Job Object + KILL_ON_JOB_CLOSE(绑 webview2)
└── 单实例锁
```

### 1.2 暴露的两个问题

**A. 提权重启死结**

"以管理员身份重启"按钮(`RelaunchElevated`)用 `ShellExecute runas` 启动新 GUI 替代自己。新 GUI **继承旧 GUI 的 Job**(已验证 `inJob=true`),旧一退出、Job handle 关闭,`KILL_ON_JOB_CLOSE` 把新 GUI 连带杀掉 → 重启失败、窗口永远不出来。

为此堆叠了 6 层 hack(单实例 `--takeover` 接管、`SW_SHOWNORMAL`、`ReleaseSingleton`、独立 WebView2 目录、sleep 等待、最后被迫移除 `KILL_ON_JOB_CLOSE` 改 OnShutdown 主动杀子),才勉强跑通。

**B. 崩溃孤儿(Wails 通病)**

为修 A 移除了 `KILL_ON_JOB_CLOSE` 后,只有"正常退出"时 `OnShutdown` 才杀 webview2;**进程崩溃 / taskkill /F 来不及跑,webview2 残留**。这是 Wails 默认行为(Wails 本身不用 Job),但对 NetSwitcher 是从"OS 兜底"退化到"无兜底"。

### 1.3 根因

`KILL_ON_JOB_CLOSE` + `ShellExecute runas` **在同一进程内**是天然冲突:runas 启动的新进程必然继承调用者的 Job,被"旧退出自动杀"误伤。**二者不可兼得**——除非把"提权重启 GUI"这个动作从架构里消除。

---

## 二、方案决策:D1

### 2.1 核心思路

**GUI 非提权 + 路由引擎作为提权子进程**。引擎寄生在 GUI 上,同生共死。

```
netswitcher.exe (GUI,非提权)
└── runas 启动 → netswitcher.exe engine (子进程,提权)
```

### 2.2 为什么选 D1(而非 Windows 服务 / 任务计划常驻)

| 维度 | D1(子进程) | C/D2(SCM 服务 / 任务计划) |
|---|---|---|
| 卸载干净 | ✅ 删 exe 即可,无残留 | ❌ 服务/任务条目残留,需手动清 |
| 引擎生命周期 | ✅ 跟 GUI,管理省心 | 引擎独立常驻 |
| Job 强绑定 | ✅ 可恢复(引擎被杀是期望) | 引擎不在 Job |
| 实现复杂度 | 中(子命令 + IPC) | 高(SCM/schtasks 装拆) |
| 调试 | 容易(直接命令行跑 engine) | 难(SESSION 0 / 附加服务) |

**决策依据**:用户最在意"卸载干净、管理省心"。D1 的引擎是 GUI 子进程,GUI 一退它就退,不会有任何后台残留。

### 2.3 取舍(诚实)

- **手动双击会弹一次 UAC**:非提权 GUI 拉起提权引擎,Windows 权限模型要求 UAC。**不可避免**。
- **开机自启可免 UAC**:任务计划 `RL HIGHEST` 启动 GUI(GUI 提权)→ GUI 用 `exec.Command` 启动引擎子进程(继承提权,无 UAC)。
- 这和**现状的 UAC 体验一致**(现状手动双击也要 UAC,只是走 RelaunchElevated)。区别是 D1 的 UAC 直接拉引擎跑起来,不再有"重启 GUI"那套死结。

---

## 三、目标架构

```
netswitcher.exe  (GUI 进程)
├── 启动方式:双击(非提权)/ 任务计划 RL HIGHEST(提权)
├── Wails GUI:配置 / 状态 / 诊断 / 日志 / 路由表
├── Job Object + KILL_ON_JOB_CLOSE  ← 恢复,绑 webview2 + 引擎子进程
├── 单实例锁(命名 mutex + show event)
└── OnStartup → 确保引擎子进程在跑
    ├── 自己已提权   → exec.Command(exe, "engine")(继承提权,无 UAC)
    └── 自己非提权   → ShellExecute runas "engine"(UAC 一次)
            │
            ▼  IPC(命名管道,复用 internal/ipc)
netswitcher.exe engine  (子进程,提权)
├── core 路由引擎:apply / netwatch(2s 轮询)/ route.exe / netsh
├── 配置文件监听(fsnotify)+ 网络变化去抖 apply
├── IPC 服务端(命名管道) ◄── GUI 连
└── 继承 GUI 的 Job → GUI 退出时被杀(期望行为)
```

### 关键属性

- **GUI 从不 runas 自己** → 永远没有 RelaunchElevated 死结。
- **引擎是子进程,被 Job 杀是期望**(它就该跟 GUI 同生共死)。
- **KILL_ON_JOB_CLOSE 恢复** → webview2 + 引擎都强绑定,崩溃也有 OS 兜底。

---

## 四、关键技术点

### 4.1 Job 继承:从"死结"变"期望"

之前死结:runas 新 GUI(要活)继承 Job → 被杀(问题)。
D1:runas 引擎(子进程,该跟 GUI 退)继承 Job → 被杀(期望)。

**同一机制,语义反转**。引擎寄生 GUI 正是用户要的"绑定"。

提权引擎能被非提权 GUI 的 Job 杀?能——`KILL_ON_JOB_CLOSE` 是 OS 级(handle 关闭触发),不走 `OpenProcess(TERMINATE)`,不受 UIPI 限制。

### 4.2 UAC 分支(GUI 启动引擎时)

```go
// 伪代码:appapi.OnStartup 里
func (a *API) ensureEngine() {
    if engineAlive() { return }  // IPC ping 通则不重启
    exe, _ := os.Executable()
    if a.elevated {
        // 已提权(任务计划启动):子进程继承提权,无 UAC
        exec.Command(exe, "engine")  // 不 detach,跟 GUI Job
    } else {
        // 非提权(双击):runas,UAC
        winutil.RunElevated(exe, "engine")
    }
}
```

### 4.3 IPC:复用 internal/ipc

项目已有完整的命名管道 IPC(`internal/ipc`,phase5 遗留,目前 dormant):
- `server.go`:引擎侧,监听命名管道,代理 core 方法
- `client.go`:GUI 侧,连管道,调用引擎方法
- `protocol.go`:请求/响应/流式协议

D1 重新启用这套,GUI 的 `appapi` 从"直连 `*core.Core`"改成"调 `ipc.Client`"。

### 4.4 引擎不需要 Job / 不需要单实例

- 引擎是 GUI 的子进程,继承 GUI Job,**自己不创建 Job**。
- 引擎不需要单实例锁(GUI 只启动一个;若引擎意外重起,管道监听可复用)。
- 引擎无 webview2,无孤儿问题。

### 4.5 引擎崩溃恢复

引擎崩溃 → GUI 的 IPC ping 失败 → GUI `ensureEngine()` 重启引擎(再 runas,UAC)。

可选增强:引擎进程挂了,GUI 检测到(管道断开)后提示用户"引擎已退出,正在重启"。

---

## 五、实施计划(4 阶段,可独立验证)

### 阶段 1:engine 子命令(让引擎能独立跑)

**目标**:`netswitcher.exe engine` 命令行能起路由引擎 + IPC 服务,不依赖 GUI。

**改动**:
- `cmd/netswitcher/cmds/engine.go`(新建):`engine` 子命令,调 `app.Start(core.Options{...})`(复用 `internal/app/app.go`,它已经做 core + IPC server 的组装)。
- `cmd/netswitcher/cmds/root.go`:`root.AddCommand(newEngineCmd(...))`。
- `internal/app/app.go`(已存在,目前 dormant):确认 `Start()` 能跑(core + ipc.Server),必要时调整日志输出(engine 模式下日志写文件 + stdout)。
- 确认 `internal/ipc/server.go` 的命名管道路径稳定(如 `\\.\pipe\NetSwitcher`)。

**验证**:
```
# 管理员 cmd 里
netswitcher.exe engine
# 另一个终端
netswitcher.exe ipc call GetStatus {}
```
能返回状态 JSON = 引擎独立跑通。

---

### 阶段 2:GUI 拉起引擎子进程

**目标**:GUI 启动时自动启动引擎,引擎跟 GUI 生命周期。

**改动**:
- `appapi/appapi.go`:
  - `OnStartup` 里加 `go a.ensureEngine()`(带重试 / 超时)。
  - 新增 `ensureEngine()`:IPC ping,不通则按 elevated 分支启动(`exec.Command` 或 `RunElevated`)。
  - 新增 `engineAlive()`:用 `ipc.Client` 发 ping,超时即视为没活。
- `pkg/winutil/elevate_windows.go`:确认 `RunElevated(exe, args)` 可用(已存在,Webview2 bootstrapper 在用)。引擎子进程需要**继承 GUI Job**,普通 `exec.Command` 自然继承;`ShellExecute runas` 也已验证继承。

**验证**:
- 双击 GUI(非提权)→ UAC → 引擎进程出现(任务管理器看 `netswitcher.exe` * 2,一个是 GUI,一个是 engine)。
- 任务计划 RL HIGHEST 启动 GUI(提权)→ 无 UAC → 引擎子进程出现。
- 关 GUI(托盘退出)→ 两个进程都消失(Job 杀)。

---

### 阶段 3:GUI 改走 IPC(不再内嵌 core)

**目标**:GUI 的所有操作通过 IPC 调引擎,GUI 自己不加载 core。

**改动**:
- `appapi/appapi.go`:
  - 移除 `a.core *core.Core` 字段 + `startEngine()`。
  - 新增 `a.client *ipc.Client`(连本地引擎)。
  - 所有方法(`GetStatus` / `GetConfig` / `SaveConfig` / `SetActiveProfile` / `ApplyNow` / `Ping` / `Tracert` / `GetRouteTable` / ...)从 `a.core.X()` 改成 `a.client.X()`。
  - `IsElevated()` 保留(GUI 自己是否提权,决定 engine 启动方式)。
  - 流式(Ping/Tracert/Logs/Status 订阅)走 IPC 流协议(`internal/ipc/stream.go`)。
- 日志:`logging.SetPipeSink` 改成连引擎的日志流(IPC),而不是本地 fanout。
- `internal/app/app.go` 的 `startEngine` 逻辑保留给 engine 子命令用;GUI 不再用。

**验证**:
- 状态页实时刷新(引擎 apply → 推 statusChanged → GUI 收到)。
- 配置保存 → 引擎 apply → 路由下发。
- 诊断页 ping/tracert 流式输出。
- 引擎杀掉 → GUI 显示"引擎离线";重启引擎 → GUI 恢复。

---

### 阶段 4:回滚问题1 的 hack 链 + 恢复 KILL_ON_JOB_CLOSE

**目标**:清掉为 RelaunchElevated 堆的所有临时代码,恢复干净的 Job 强绑定。

**改动见第六节"代码取舍清单"**。

**验证**:
- 双击 → UAC → 引擎跑 → 窗口显示 → 托盘左右键正常。
- 开机自启(任务计划)→ 免 UAC → 一切自动。
- 强杀 GUI(taskkill /F)→ webview2 + 引擎都被 Job 清掉(任务管理器确认无残留)。
- 不存在 `--takeover` / `RelaunchElevated` / 独立 WebView2 目录等任何痕迹。

---

## 六、代码取舍清单

### 6.1 回滚(问题1 的 hack 链,阶段 4 删除)

这些是为"RelaunchElevated 提权重启"堆的临时代码,D1 下 GUI 不 runas 自己,**全部删除**:

| 文件 | 临时代码 | 处理 |
|---|---|---|
| `cmd/netswitcher/cmds/root.go` | `takeoverFlag` 变量、`--takeover` flag、`MarkHidden`、RunE 传 takeover | 删 |
| `cmd/netswitcher/cmds/gui.go` | `runGUI(version, takeover)` 签名、接管等待循环、sleep、`inJob` 日志、`runGUI start` 日志、`newGUICmd` 的 takeover flag、`time`/`slog`/`os` import | 恢复成 `runGUI(version)`,删所有诊断日志 |
| `pkg/winutil/elevate_windows.go` | `RelaunchElevated` 带 `--takeover` + `SW_SHOWNORMAL`、`relaunchRunas(showCmd int32)` | 删 `RelaunchElevated`;`relaunchRunas` 恢复成 `SW_HIDE`(只给 Webview2 bootstrapper 用) |
| `pkg/winutil/singleton_windows.go` | `ReleaseSingleton()` | 删 |
| `pkg/winutil/job_windows.go` | 移除 `KILL_ON_JOB_CLOSE`、`KillChildProcesses()`、`InJob()`、`procIsProcessInJob`、`strings` import | **恢复 `KILL_ON_JOB_CLOSE`**(SetInformationJobObject);删 KillChildProcesses / InJob |
| `pkg/winutil/job_other.go` | `InJob` / `KillChildProcesses` stub | 删 |
| `gui.go` | `Options.Takeover` 字段 | 删 |
| `gui_cgo.go` | `windowsOptions(takeover)`(独立 WebView2 目录)、`OnDomReady`、`OnBeforeClose` 的 pid/quitting 日志、`OnShutdown` 的 `KillChildProcesses()` 调用、`filepath`/`winutil` import | 全删;`OnShutdown` 整个回调可删(Job 自动兜底);`Windows: &windows.Options{}` 恢复 |
| `appapi/appapi.go` | `RelaunchElevated()` 方法、`ReleaseSingleton` 调用 | 删 |
| `frontend/src/App.svelte` | `relaunchElevated()` 函数 + "以管理员身份重启"按钮 + 首启提权弹窗 | 改成"启动引擎"(或自动 ensureEngine,按钮删) |
| `frontend/wailsjs/go/appapi/API.{d.ts,js}` | `RelaunchElevated` 绑定 | 删 |

### 6.2 保留(真 bug 修复 / 增强,与架构无关)

这些和提权死结无关,是排查过程里顺带修的真问题,**保留**:

| 文件 | 改动 | 为什么保留 |
|---|---|---|
| `internal/routeengine/metric.go` | `name=` → `interface=`(netsh set interface 参数名) | 真 bug,引擎跑 netsh 也需要 |
| `internal/ifacemgr/ifacemgr.go` | `Interface.Metric` 字段 | 状态页显示用 |
| `internal/ifacemgr/ifacemgr_windows.go` | `Metric: int(a.Ipv4Metric)` | 同上 |
| `frontend/wailsjs/go/models.ts` | `Interface.Metric` | 同上 |
| `frontend/src/pages/Status.svelte` | 接口卡片 Metric 行 | 同上 |
| `frontend/src/pages/Profiles.svelte` | "未配置·系统 WLAN" 显示(消除"系统默认"误导) + 下拉框监听 statusChanged 刷新 | 真 UX bug |

### 6.3 新增(D1 架构需要)

| 文件 | 内容 |
|---|---|
| `cmd/netswitcher/cmds/engine.go`(新) | `engine` 子命令:调 `app.Start` 跑 core + IPC server |
| `appapi/appapi.go` 改造 | `ensureEngine()` / `engineAlive()`;`a.client *ipc.Client` 替代 `a.core` |
| `internal/app/app.go` | 从 dormant 唤醒(已经是 core + ipc 组装,基本可直接用) |
| `internal/ipc/*` | 从 dormant 唤醒,确认协议完整覆盖 GUI 所有方法 |

---

## 七、风险与缓解

| 风险 | 缓解 |
|---|---|
| 引擎启动有延迟,GUI 首次状态加载慢 | GUI 启动时显示"正在连接引擎...",IPC 连上后刷新 |
| 引擎崩溃,GUI 不知道 | IPC 连接断开 → GUI 显示"引擎离线" → 自动 `ensureEngine` 重启 |
| 双击 UAC 弹窗用户取消 | GUI 进入"只读模式"(能看配置/路由表,不能改),提示"引擎未运行,点击启动" |
| 引擎子进程没继承 Job(罕见) | 验证步骤里确认(taskkill GUI 后引擎也消失) |
| IPC 协议覆盖不全(某些方法没代理) | 阶段 3 前先对齐 `appapi` 所有方法和 `ipc.Server` 的 handler 表 |
| 引擎和 GUI 同时读 config.json | 已有 `config.Watcher` + 保存时的 suppress 机制,引擎监听文件变化即可 |

---

## 八、验证矩阵

每个阶段完成后的验收用例:

**阶段 1**(engine 独立):
- [ ] `netswitcher.exe engine` 能起,日志写文件
- [ ] `netswitcher.exe ipc call GetStatus {}` 返回 JSON

**阶段 2**(GUI 拉引擎):
- [ ] 双击 GUI → UAC → 两个 netswitcher.exe 进程
- [ ] 任务计划 RL HIGHEST 启动 → 无 UAC → 两个进程
- [ ] 关 GUI(托盘退出)→ 两进程消失

**阶段 3**(IPC 打通):
- [ ] 状态页实时刷新(拔插网线 → 接口卡片变)
- [ ] 配置页保存 → 状态页"已下发路由"更新
- [ ] 诊断页 ping baidu.com 流式输出
- [ ] 杀引擎 → GUI 提示 → 重启引擎 → 恢复

**阶段 4**(回滚 + Job 恢复):
- [ ] `git diff` 确认第六节 6.1 的文件都已清理
- [ ] `taskkill /F /PID <gui>` → webview2 + engine 都消失(任务管理器)
- [ ] 不存在 `--takeover` / RelaunchElevated 等代码

**端到端**:
- [ ] 默认路由走 WLAN(启用有线后流量仍走 WLAN,`tracert baidu.com` 第一跳是 WLAN 网关)
- [ ] 168.168/16、172.16/16 走以太网 3(`tracert 168.168.x.x` 第一跳以太网网关)
- [ ] 开机自启:重启系统 → 登录 → 路由自动维护(免 UAC)

---

## 九、后续可选优化(非本方案范围)

- **引擎 watchdog**:引擎崩溃自动重启(GUI 侧 ensureEngine 已覆盖,但可做更积极的健康检查)。
- **引擎多用户会话隔离**:目前 `Local\` 命名空间 + 命名管道,多用户同机登录会有冲突(罕见场景,暂不处理)。
- **升级流程**:新版 exe 替换旧版,需重启 GUI + 引擎(任务计划重启或 GUI 提示)。
- **回退到 SCM 服务(方案 C)**:若未来需要"登录前也维护路由"(如 RDP 登录界面),D1 不够,再考虑 C。当前需求(登录后维护)D1 满足。

---

## 附:决策记录

- **2026-07-06**:确认从"GUI 内嵌引擎"(commit `75a74a7`)回退到拆分架构。
- 候选过 C(SCM 服务)、D2(任务计划常驻)、D1(GUI 子进程)。
- 选 D1:用户最在意"卸载干净、管理省心",D1 引擎寄生 GUI,删 exe 即净。
- 接受代价:手动双击 UAC 一次(和现状一致),开机自启免 UAC。
