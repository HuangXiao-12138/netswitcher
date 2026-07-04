# NetSwitcher

内外网路由管理工具 —— 让 Windows 双网卡（内网以太网 + 外网 Wi-Fi）按网段自动分流，并常驻维护。

> 本仓库按 `docs/NetSwitcher-技术方案.md` 的 Phase 0–7 实现。**Phase 0–7 全部完成。**

## 解决什么问题

机器同时连着内网（以太网，无 Internet）和外网（Wi-Fi，有 Internet）时，让指定网段（如 `168.168.0.0/16` 内网、`172.16.0.0/16` 终端）走以太网，其余流量（含默认路由）走 Wi-Fi。手动 `route -p` 在 Wi-Fi 重连、网卡 index 变化、DHCP 续约后经常失效；NetSwitcher 用常驻服务监听网络变化、自动重新下发 runtime 路由（**绝不带 `-p`**），并把"哪些网段走哪块网卡"的规则做成图形配置。

## 单二进制多角色

```
netswitcher.exe service install|uninstall|start|stop|run   # Windows 服务
netswitcher.exe gui                                         # 桌面 GUI
netswitcher.exe apply [--dry-run]                           # 应用一次后退出（调试）
netswitcher.exe dump                                        # 打印接口/配置（调试）
netswitcher.exe ipc call <method> [json]                    # 命名管道自测（隐藏）
netswitcher.exe --help
```

服务运行时还监听命名管道 `\\.\pipe\NetSwitcher`（行分隔 JSON 协议）。

## 快速开始

### 从源码构建

依赖：**Go 1.22+**、**Node.js 18+**、**MinGW-w64 (gcc)**（仅完整 GUI 构建需要）。

```bash
make build          # 完整构建：npm build + CGO go build → netswitcher.exe
make build-cli      # 仅服务/CLI（CGO_ENABLED=0，无需 gcc）
make test           # 单测（含 -race）
make dev            # Wails 热重载开发
```

Windows PowerShell：`.\build.ps1`（完整）或 `.\build.ps1 -CliOnly`。

> Wails 的绑定生成器在某些 MinGW 工具链下其临时 `wailsbindings.exe` 会被 Windows 加载器拒绝（已知 CGO 工具链怪癖）。本仓库的 `frontend/wailsjs/` 已手写完成，`make build` 直接走 `go build`，不依赖 `wails build`/`wails generate`。

### 安装为服务（需管理员）

```powershell
.\netswitcher.exe service install   # 注册为开机自启服务（自动重启 on failure）
.\netswitcher.exe service start
.\netswitcher.exe gui               # 打开配置/诊断界面
```

卸载：

```powershell
.\netswitcher.exe service stop
.\netswitcher.exe service uninstall
```

或使用 NSIS 安装包（`makensis build/windows/installer.nsi` 生成 `dist/NetSwitcher-Setup.exe`）。

## 配第一条规则

1. 启动 GUI（`netswitcher.exe gui`）。
2. 进入 **配置** 页 → **+ 新建配置**。
3. 在规则表里加一行：
   - 目标 CIDR：`168.168.0.0/16`
   - 接口：选你的内网网卡（如"以太网"）
   - 网关：`auto`（自动取该网卡当前默认网关）
   - 启用：勾选
4. （可选）设置 **默认路由网卡** 为 Wi-Fi，让其它流量走外网。
5. **保存** → **设为活动**。3 秒内 **状态** 页会反映变化。

## 数据目录

`%ProgramData%\NetSwitcher\`：`config.json`（配置）、`state.json`（上次下发的路由）、`logs\netswitcher.log`（按天滚动，保留 7 天）。

## 常见问题：路由没生效怎么排查

1. **服务在跑吗？** GUI 顶栏应显示"服务在线"。若否，点横幅里的"以管理员身份启动服务"。
2. **规则被跳过？** 状态页底部"跳过的规则"会写明原因：
   - `interface not found`：接口名拼错，或网卡当前未连接（等连上后自动重试）。
   - `no IPv4 gateway on …`：DHCP 还没拿到网关（重连后自动重试）。
3. **冲突？** 状态页顶部"冲突告警"会标出 VPN 适配器或外部覆盖（NetSwitcher 不会主动覆盖 VPN 路由）。
4. **实际走哪？** 诊断页 `tracert <目标>` 看第一跳；路由表页按来源着色（本工具/系统/疑似 VPN）。
5. **看日志：** 日志页实时滚动，或打开 `%ProgramData%\NetSwitcher\logs\netswitcher.log`。每条 apply 都记录 `reason`（startup / network_change / config_change）。

## 进度

| Phase | 内容 | 状态 |
|---|---|---|
| 0 | 项目骨架与构建 | ✅ |
| 1 | 配置 + 接口枚举 | ✅ |
| 2 | 路由引擎 | ✅ |
| 3 | 网络监听 + 自动应用 | ✅ |
| 4 | Windows 服务化 | ✅ |
| 5 | IPC（命名管道） | ✅ |
| 6 | GUI（Wails + Svelte） | ✅ |
| 7 | 打包与收尾 | ✅ |

详细技术决策见 `docs/NetSwitcher-技术方案.md`，使用说明见 `docs/USER-MANUAL.md`。

## 测试

`go test -race ./...` 覆盖配置校验、接口名匹配、路由 diff/apply、网络变化检测、防抖、IPC 协议/扇出、服务配置。VM 双网卡集成测试矩阵见方案 §13.2。

## 许可

暂未指定（内部使用）。
