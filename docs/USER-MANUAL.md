# NetSwitcher 用户手册

NetSwitcher 帮你在「内网以太网 + 外网 Wi-Fi」双网卡机器上自动分流：你只管说"哪些网段走哪块网卡"，剩下的（默认路由、metric、Wi-Fi 重连后重下发）它来维护。

---

## 1. 安装

### 方式 A：安装包（推荐）

下载 `NetSwitcher-Setup.exe`，双击一路下一步。安装包会：

1. 把 `netswitcher.exe` 放进 `C:\Program Files\NetSwitcher\`。
2. 创建开始菜单 / 桌面快捷方式（指向 GUI）。
3. 注册并启动 Windows 服务（开机自启）。
4. （旧 Win10）按需安装 WebView2 运行时。

安装完成后，从开始菜单打开 **NetSwitcher** 即可。

### 方式 B：免安装（便携）

```powershell
netswitcher.exe service install   # 需管理员
netswitcher.exe service start
netswitcher.exe gui
```

## 2. 配置第一条规则

打开 GUI，进入左侧 **配置** 页：

1. 点 **+ 新建配置**，给个名字（如"办公区"）。
2. 在规则表 **+ 添加规则**：
   - **目标 CIDR**：`168.168.0.0/16`（内网服务器段）
   - **接口**：下拉选你的内网网卡（如"以太网"）
   - **网关**：`auto`（自动取该网卡当前网关；也可填具体 IP）
   - **Metric**：`1`（小 = 优先）
   - **启用**：勾选
3. 再加一条 `172.16.0.0/16` → 同一内网网卡（如有终端段）。
4. **默认路由网卡** 下拉选 Wi-Fi（让其它流量走外网）。
5. **自动管理接口跃点数** 保持勾选（让默认路由确实走 Wi-Fi）。
6. 点 **保存**，再点 **设为活动**。

几秒内 **状态** 页会刷新：接口卡片、已下发路由表、最近一次 apply 结果。

> 任何时候网卡重连、插拔网线、DHCP 续约，服务都会在 ~2–5 秒内自动重新下发路由，无需手动干预。

## 3. 切换配置（Profile）

不同场景（办公 / 出差 / 家庭）可有不同规则集。在 **配置** 页左侧选另一个 profile → **设为活动**，路由即按新规则重下发。GUI 顶栏始终显示当前活动 profile 名。

## 4. 诊断

**诊断** 页：

- 选 `ping` 或 `tracert`，输入目标 IP/域名，点 **运行**。
- 输出实时滚动。`tracert` 第一跳告诉你流量实际走了哪块网卡的网关。
- 点 **停止** 中断（`tracert` 可能较慢，已限制 ≤20 跳 / 90 秒）。

**路由表** 页：

- 来自 `Get-NetRoute` 的完整 IPv4 路由表，按来源着色：
  - 🟢 **本工具**：NetSwitcher 下发的
  - 🟣 **疑似 VPN**：落在 VPN/虚拟网卡上
  - ⚪ **系统**：其它
- 顶部搜索框可按目标/下一跳/接口过滤。

## 5. 状态页解读

- **接口卡片**：每块网卡一张，显示名称、是否连接、IPv4、网关、类型、Index。
- **已下发路由**：当前由本工具管理的路由（目标 → 下一跳 → 接口 → metric）。
- **跳过的规则**：写明哪条规则因什么原因没下发（接口名拼错、网卡没连、无网关）。下一轮网络变化会自动重试。
- **冲突告警**：检测到 VPN 适配器在线或目标被外部覆盖时，顶部横幅提示（**不会**自动覆盖 VPN 路由）。
- **底部**：最近一次 apply 的时间、原因（`startup` / `network_change: …` / `config_change` / `ipc`）。

## 6. 日志

**日志** 页实时滚动所有服务日志。下拉选级别（debug/info/warn/error），搜索框过滤。日志同时写入：

```
%ProgramData%\NetSwitcher\logs\netswitcher.log
```

按天滚动，单文件超过 50MB 也会滚动，保留 7 天。

## 7. 服务未运行怎么办

GUI 顶栏显示"服务未运行"并出现红色横幅时，点 **以管理员身份启动服务**（会弹 UAC，同意即可）。横幅消失后服务恢复在线。

手动管理（PowerShell 管理员）：

```powershell
netswitcher.exe service start     # 启动
netswitcher.exe service stop      # 停止
netswitcher.exe service uninstall # 卸载
```

服务被设为 **开机自启 + 失败自动重启**（10 秒后）。即使没人登录，路由也会在系统启动时由 SYSTEM 服务下发好。

## 8. 卸载

- 安装包：开始菜单 → "卸载 NetSwitcher"，或"添加/删除程序"。卸载会先停服务、移除服务注册，然后询问是否删除配置与状态。
- 便携：先 `netswitcher.exe service stop && service uninstall`，再删 exe。

## 9. 故障排查清单

| 现象 | 排查 |
|---|---|
| 路由没生效 | 状态页"已下发路由"里有没有？没有看"跳过的规则"。 |
| 内网还是不通 | 诊断页 `tracert 内网IP`，第一跳应是内网网卡网关；不是则检查规则接口/网关。 |
| 外网断了 | 默认路由网卡设对了吗？metric 管理开了吗？状态页接口卡看 Wi-Fi 是否连接。 |
| Wi-Fi 重连后断 | 服务在跑吗？日志里应有 `network_change` 的 apply 记录。 |
| 配置保存报错 | 字段级错误会标红（CIDR 非法、网关非法、重复规则等）。 |
| 与 VPN 冲突 | 状态页"冲突告警"会标出；本工具不主动覆盖，关掉 VPN 或忽略即可。 |

## 10. 配置文件格式

`%ProgramData%\NetSwitcher\config.json`：

```json
{
  "version": 1,
  "activeProfile": "office",
  "profiles": [{
    "id": "office",
    "name": "办公区",
    "rules": [
      {"id": "r1", "destination": "168.168.0.0/16", "viaInterface": "以太网", "viaGateway": "auto", "metric": 1, "enabled": true}
    ],
    "defaultRouteInterface": "WLAN",
    "autoManageMetrics": true,
    "metricPolicy": {"preferredInterface": "WLAN", "preferredMetric": 10, "othersMetric": 50}
  }],
  "logLevel": "info"
}
```

服务监听该文件变化，外部编辑保存后 2 秒内自动 reload + 重新 apply。
