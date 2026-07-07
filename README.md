# NetSwitcher

内外网路由管理工具 —— 让 Windows 双网卡（内网以太网 + 外网 Wi-Fi）按网段自动分流，并常驻维护。

> 解决"手动 `route -p` 在 Wi-Fi 重连、网卡 index 变化、DHCP 续约后失效"的痛点：NetSwitcher 常驻监听网络变化，自动重新下发 runtime 路由（**绝不带 `-p`**），路由始终跟着你的配置走。

## 截图

**状态页** — 接口卡片、已下发路由、冲突告警：

![状态页](docs/images/status.webp)

**配置页** — 路由规则、域名解析规则、高级设置：

![配置页](docs/images/profiles.webp)

## 特性

- 🎯 **按网段分流**：CIDR 规则匹配，指定网段走指定网卡，其余走默认
- 🔄 **自动维护**：网络变化（Wi-Fi 重连、网卡增减、DHCP 续约）2 秒内检测，去抖后自动重下发
- 🛡️ **常驻托盘**：关窗口缩到托盘继续工作，不占任务栏；开机自启免 UAC
- 📊 **可视化**：接口状态、路由表（按来源着色）、冲突告警（VPN / 外部覆盖）
- 🔧 **诊断工具**：内置 ping / tracert 流式输出
- 📝 **实时日志**：级别筛选 + 关键词过滤
- 🚀 **自动更新**：启动自动检查新版本，顶栏红点提示，一键升级（v0.0.2+）
- 🚫 **不带 `-p`**：只下发 runtime 路由，不污染系统持久路由表

## 下载安装

从 [Releases](https://github.com/HuangXiao-12138/netswitcher/releases/latest) 下载 `NetSwitcher-<version>-x86_64-Portable.zip`，解压双击 `NetSwitcher.exe` 即可，无需安装。

> 首次启动会请求管理员权限（修改路由需要）。同意后在「设置 → 开机自启」开启，下次登录自动以管理员启动，免每次 UAC。

## 快速上手

1. **启动**：双击 `NetSwitcher.exe` → 同意 UAC → 提权运行。
2. **配置规则**：「配置」页 → 添加 profile → 填规则（目标 CIDR + 接口 + 网关）→ 保存 → 设为活动。
3. **开机自启（推荐）**：「设置」页 → 开启"开机自启"→ 之后登录自动启动。
4. **常驻**：关窗口（✕）→ 缩到托盘继续维护路由；托盘右键"退出"才真正结束。

## 页面功能

| 页 | 功能 |
|---|---|
| 状态 | 接口卡片（up/down、IPv4、网关、类型）、已下发路由表、"立即重新应用"、冲突告警（VPN/外部覆盖）、跳过/错误列表 |
| 配置 | profile 列表 + 规则表编辑（CIDR / 接口下拉 / 网关 / metric / 启用）、默认路由网卡、metric 策略、字段级校验 |
| 路由表 | `Get-NetRoute` 全表，按来源着色（🟢本工具 / 🟣疑似 VPN / ⚪系统），可搜索 |
| 诊断 | ping / tracert 流式输出，可停止 |
| 日志 | 实时日志流 + 级别筛选 + 关键词过滤 |
| 设置 | 开机自启、日志级别、检查更新、一键升级、打开日志目录、版本/路径 |

## 工作原理

```
NetSwitcher.exe（提权运行，单进程）
├── Wails GUI（无边框自绘顶栏 + 系统托盘 + 单实例锁）
├── core 路由引擎（进程内，调 route.exe / netsh）
├── netwatch（2s 轮询，网络变化自动 1500ms 去抖重下发）
├── Job Object（父进程退出，所有子进程一起回收）
└── 任务计划（可选，登录时自动以管理员启动，免 UAC）
```

- **没有 Windows 服务**：路由引擎跑在 GUI 进程里，关窗口（缩托盘）后继续维护路由；托盘"退出"才结束。
- **路由修改需要管理员权限**：必须提权运行。
- **代价**：只在登录之后维护路由（登录前 / 注销时不工作）—— 对个人台式机一般无所谓。

> 实现规格见 [`docs/NetSwitcher-技术方案.md`](docs/NetSwitcher-技术方案.md)，使用说明见 [`docs/USER-MANUAL.md`](docs/USER-MANUAL.md)。当前架构（GUI 内嵌引擎 + 托盘）与方案文档最初的"Windows 服务 + IPC"不同，已演化。

## 自动更新

v0.0.2 起支持启动自动检查更新：

- 启动后后台静默检查 GitHub Release，有新版本时**顶栏出现蓝色圆点**提示
- 点击红点 → 弹出升级对话框（版本对比 + 更新内容 + 进度条）→ 一键升级
- 下载完成自动倒计时重启（可点「立即重启」跳过）
- 网络失败 / 无更新 / 开发版本时**不打扰**（顶栏无提示）
- 自动读取系统代理，代理环境下也能正常下载

## 路由没生效怎么排查

1. **提权了吗？** 顶栏药丸应显示"路由引擎在线"。否则走"以管理员身份重启"。
2. **规则被跳过？** 状态页"跳过的规则"写明原因（接口未连接、无网关）—— 网卡连上后自动重试。
3. **冲突？** 状态页顶部标出 VPN 适配器 / 外部覆盖（本工具不主动覆盖 VPN）。
4. **实际走哪？** 诊断页 `tracert <目标>` 看第一跳。
5. **看日志**：日志页实时滚动，或 `%ProgramData%\NetSwitcher\logs\netswitcher.log`。

## 数据目录

`%ProgramData%\NetSwitcher\`：`config.json`（配置）、`state.json`（上次下发的路由）、`logs\netswitcher.log`（按天滚动，保留 7 天）。

## 构建（开发者）

依赖：**Go 1.22+**、**Node.js 18+**、**MinGW-w64 (gcc)**（CGO，链接 WebView2）。

```bash
make build          # npm build + CGO go build → NetSwitcher.exe
make build-cli      # 仅服务/CLI（CGO_ENABLED=0，无需 gcc，无 GUI）
make test           # 单测（含 -race）
make icon           # 从 build/windows/icon.ico 重新生成 resource.syso
```

Windows PowerShell：`.\build.ps1`（完整）或 `.\build.ps1 -CliOnly`。

> - GUI 必须带 `-tags desktop,production`，否则 Wails 运行时报"missing build tags"。
> - 必须带 `-ldflags "-H windowsgui"`，否则双击弹黑控制台。
> - `frontend/wailsjs/` 是**手写并提交**的，`make build` 走 `go build`，不依赖 `wails` CLI（某些 MinGW 工具链下 `wailsbindings.exe` 会被 Windows 加载器拒绝）。
> - exe 图标通过 `rsrc` 把 `build/windows/icon.ico` 编进 `cmd/netswitcher/resource.syso`（`make icon` 重生成）。

## 测试

`go test -race ./...` 覆盖配置校验、接口名匹配、路由 diff/apply、网络变化检测、去抖、IPC 协议、日志扇出、更新检查（错误分类、下载解压、进度回调）。

## 已知边界

- 仅 Windows 10/11 x64（用 WebView2；Win10 需装运行时，Win11 自带）。
- 仅 IPv4（IPv6 字段预留但不处理）。
- 不与 VPN 客户端深度集成（只检测冲突、告警）。
- 嵌入式架构只在用户登录后维护路由（登录前 / 注销时不工作）。

## 许可

MIT — 随意使用、修改、分发（含商用），保留版权声明即可。详见 [LICENSE](LICENSE)。
