# UFI-Gate

为 [UFI-TOOLS](https://github.com/kanoqwq/UFI-TOOLS/tree/http-server-version) 随身 WiFi 提供网页认证、设备准入与流量配额管理。

开放热点连接后，访客先完成认证，再获得上网许可。管理后台独立运行，采用 Fluent / WinUI 3 视觉风格，适配手机、平板和电脑。

**当前版本：`1.0.0-beta.1`，开发测试版。** 已完成本机逻辑及浏览器验证；真机网络拦截、固件兼容性、真实推送与第三方验证码联调仍待验收。

## 下载与安装

前往 [Releases](https://github.com/zhaosaojis-debug/UFI-Gate/releases) 下载 `UFI-Gate-1.0.0-beta.1-arm64.js`。本插件包含静态 ARM64 守护服务，安装时无需额外下载运行时。

运行要求：**Rooted ARM64 Android 随身 WiFi、UFI-TOOLS 设备端完整版（`http-server-version`）、可用的 Root Shell，以及所需的 iptables / ip6tables 内核模块。**

1. 登录 UFI-TOOLS，在插件管理中导入 `.js`，保存并刷新。
2. 打开“UFI-Gate 网闸”，检测环境，核对热点接口和网关，安装服务。
3. 打开独立后台，初始化管理员账号，配置认证方式、上网账号、配额与推送。
4. 保留 Root Shell 或 ADB 恢复通道，开启认证，将热点设为 OPEN，并启用、验证 AP 隔离。
5. 用第二台未受信任的设备完成 [真机验收](docs/INSTALL.md#真机验收当前待完成)，再开放给访客。

所有插件合计受宿主 5 MiB 存储限制；本插件约 3.45 MiB。完整步骤、配置口径和故障处理见 [安装与管理指南](docs/INSTALL.md)。

## 功能

| 模块 | 能力 |
| --- | --- |
| 管理后台 | 独立管理员登录、设备看板、实时速率与用量排行、日志、浅色与深色主题 |
| 设备管理 | MAC / IP 识别、备注、放行、撤销、封禁、永久信任、管理员解禁 |
| 流量与配额 | 上下行统计、每日 / 历史累计 / 会话 / 固定周期配额、提前限速、满额限速或断网 |
| 门户认证 | 上网账号密码、本地图形验证码、hCaptcha / Turnstile / 自定义验证接口、管理员审批 |
| 审批推送 | 企业微信、飞书、通用 Webhook、SMTP 邮件；带有效期的审批链接 |
| 网络准入 | 未认证 IPv4 默认拒绝、DNS 门户应答、HTTP 跳转；开启期间阻断热点 IPv6 |
| 访问策略 | 域名 / IPv4 黑白名单、白名单免认证、警告页与联系信息定制 |
| 生命周期 | 独立常驻、开机启动、停止 / 卸载 / 崩溃恢复、宿主停用检测 |

## 使用边界

- **HTTPS / HSTS 无法无证书警告地透明跳转。** 未认证的 HTTPS、QUIC 与 APP 外网连接会被阻断；认证页由系统“登录网络”、HTTP 请求或本地入口打开。
- 域名过滤基于 DNS 与 IPv4 地址，属于尽力过滤。已认证设备的加密 DNS、VPN、缓存 IP 与共享 CDN 会影响准确性；本版本不提供应用层审计。
- 厂商硬件 NAT / 加速、AP 隔离、随机 MAC、固件内核模块必须在实际设备验证；当前仅支持一套热点接口和 IPv4 上网。
- 流量每 5 秒采样，包含 IP 开销与重传，可能超过配额一个采样窗口；不是运营商计费账本。限速通过字节型 hashlimit 丢包实现。
- 远程审批地址需经 VPN 或 HTTPS 隧道可达；机器人 Webhook 或邮件不会自动让内网设备暴露到公网。
- 停止或恢复后恢复原始网络准入；如果热点已设为 OPEN，需要另行恢复热点密码。

这些限制及第三方服务配置详见 [安装与管理指南](docs/INSTALL.md#实现与边界)。

## 紧急恢复

在 UFI Root Shell 或 ADB / su 中执行：

```sh
sh /data/ufi-gate/service.sh recover
```

如服务二进制不可执行，可使用仓库内 [emergency-recover.sh](emergency-recover.sh)。恢复操作只移除本插件的 `UFIG_` 链和接入，不清空全局防火墙。建议首次启用前准备恢复通道。

## 开发与构建

Go 1.24 或更新版本、Python 3、Node.js；Go 依赖已 vendored。

```sh
go test -mod=vendor -race ./...
go vet -mod=vendor ./...
python3 build.py --out ./dist
```

输出：单文件 `.js` 安装器、静态 ARM64 二进制、构建信息。Go 1.24.2 是本次发布使用的工具链。

本地 UI 演示不操作防火墙：

```sh
go build -mod=vendor -buildvcs=false -o ufi-gate-demo .
./ufi-gate-demo --demo --dir ./demo-state
```

浏览器集成测试需安装 Playwright 及 Chromium，并先生成 `ufi-gate-demo`：

```sh
npm install --no-save playwright
npx playwright install chromium
node tests/ui-test.cjs
```

可通过 `CHROME_PATH` 指定已有浏览器，通过 `PLUGIN_PATH` 指定构建的安装器以验证内嵌负载。上述演示及规则执行器测试不能替代真机验收。

## 测试与发布

首版已通过 **21 项 Go 测试及 race 检查**、`go vet`、网页与 Shell 语法检查；完成 390 / 834 / 1440 三个宽度共 30 个后台布局检查，以及账号、验证码、申请审批、封禁和解禁等浏览器流程。

详见 [首版测试记录](docs/validation-beta.1.json) 与 [发布说明](docs/releases/v1.0.0-beta.1.md)。记录描述本机验证，真机验收及实际服务联调状态明确为待完成。

GitHub Actions 对推送和 PR 运行逻辑与语法检查。首版发布工作流通过手动运行生成附件、校验值和预发布 Release；遇到已有相同版本会停止，避免覆盖附件。

## 目录

| 路径 | 内容 |
| --- | --- |
| `main.go` | 服务 API、账户、认证、配额、审批与推送 |
| `firewall.go` / `dns.go` | IPv4 准入、计数、限速、DNS 与域名规则 |
| `web/` | 管理后台和访客认证页 |
| `plugin.template.js` | UFI-TOOLS 插件与内嵌服务安装器模板 |
| `service.sh` | 常驻、开机接入、停止与卸载 |
| `tests/` / `main_test.go` | 浏览器和逻辑测试 |
| `docs/` | 安装指南、测试记录与版本说明 |
| `scripts/` | Release 打包与校验 |

## 许可与致谢

原创代码按 [MIT License](LICENSE) 提供。`vendor/` 内的 Go 密码库按其原有许可证分发。感谢 [UFI-TOOLS](https://github.com/kanoqwq/UFI-TOOLS) 提供宿主接口；本项目是独立插件，与上游不存在官方隶属关系。
