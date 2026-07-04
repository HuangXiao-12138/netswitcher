# NetSwitcher

内外网路由管理工具 —— 让 Windows 双网卡（内网以太网 + 外网 Wi-Fi）按网段自动分流。

> 本仓库按 `docs/NetSwitcher-技术方案.md` 的 Phase 0–7 实现，当前进度：**Phase 0（项目骨架）已就绪**。

## 解决什么问题

机器同时连着内网（以太网，无 Internet）和外网（Wi-Fi，有 Internet）时，让指定网段（如 `168.168.0.0/16` 内网、`172.16.0.0/16` 终端）走以太网，其余流量（含默认路由）走 Wi-Fi。手动 `route -p` 在 Wi-Fi 重连、网卡 index 变化、DHCP 续约后经常失效；NetSwitcher 用常驻服务监听网络变化、自动重新下发 runtime 路由。

## 单二进制多角色

```
netswitcher.exe service install|uninstall|start|stop|run   # Windows 服务
netswitcher.exe gui                                         # 桌面 GUI
netswitcher.exe apply                                       # 应用一次后退出（调试）
netswitcher.exe dump                                        # 打印接口/配置/路由（调试）
netswitcher.exe --help
```

## 构建

依赖：Go 1.22+、Node.js 18+、MinGW-w64（gcc，仅完整 GUI 构建需要）。

```bash
make build          # 完整构建（含 GUI，需要 gcc）
make build-cli      # 仅服务/CLI（CGO_ENABLED=0，无需 gcc）
make test           # 单测
make dev            # Wails 热重载开发
```

Windows PowerShell：`.\build.ps1` 或 `.\build.ps1 -CliOnly`。

数据目录：`%ProgramData%\NetSwitcher\`（`config.json` / `state.json` / `logs/`）。

## 状态

| Phase | 内容 | 状态 |
|---|---|---|
| 0 | 项目骨架与构建 | ✅ |
| 1 | 配置 + 接口枚举 | ⏳ |
| 2 | 路由引擎 | ⏳ |
| 3 | 网络监听 + 自动应用 | ⏳ |
| 4 | Windows 服务化 | ⏳ |
| 5 | IPC（命名管道） | ⏳ |
| 6 | GUI（Wails + Svelte） | ⏳ |
| 7 | 打包与收尾 | ⏳ |

详见 `docs/NetSwitcher-技术方案.md`。
