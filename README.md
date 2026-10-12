<div align="center">

# UFI-Gate

WiFi Captive Portal · 设备准入 · 流量配额

![banner](https://github.com/nanbeiluduo/UFI-Gate/raw/main/images/banner.png)

</div>


为 [UFI-TOOLS](https://github.com/kanoqwq/UFI-TOOLS/tree/http-server-version) 随身 WiFi 提供网页认证、设备准入与流量配额管理。

开放热点连接后，访客先完成认证，再获得上网许可。管理后台独立运行，采用 Fluent / WinUI 3 视觉风格，适配手机、平板和电脑。

**当前版本：`1.0.4`。** 新增门户外观自定义：网站 Logo 与认证页 Logo 可在后台上传替换。已在中兴 MU3351 / V50（`armv8l`）真机完成端到端验证。

## 下载与安装

### 安装包架构选择

在 UFI Root Shell 运行 `uname -m`，按当前系统执行架构选择：

| 输出 | 下载文件 |
| --- | --- |
| `armv7l` / `armv8l` | `UFI-Gate-1.0.4-arm32.js` |
| `aarch64` / `arm64` | `UFI-Gate-1.0.4-arm64.js` |

ARM32 使用 `GOARCH=arm GOARM=7,softfloat CGO_ENABLED=0`，不依赖设备的动态链接器或硬浮点支持；要求 ARMv7 或更高的 32 位执行环境。`armv8l` 应选择 ARM32 包，不能通过跳过架构检测运行 ARM64 二进制。其他架构会在写入服务前被拒绝。

只保留一个 UFI-Gate 插件：删除旧插件条目后导入对应的新包，保存并刷新，再检测环境并安装 / 更新服务。若旧服务正在运行，更新会暂时恢复网络并关闭认证，完成后需检查配置并重新开启。已有 `/data/ufi-gate` 状态和流量历史保留；不要删除状态目录。

前往 [Releases](https://github.com/nanbeiluduo/UFI-Gate/releases) 下载与系统匹配的 `arm32.js` 或 `arm64.js` 安装包。本插件包含静态 ARM32 / ARM64 守护服务，安装时无需额外下载运行时。

运行要求：**Rooted ARM32 / ARM64 Android 随身 WiFi、UFI-TOOLS 设备端完整版（`http-server-version`）、可用的 Root Shell，以及所需的 iptables / ip6tables 内核模块。**

1. 登录 UFI-TOOLS，在插件管理中导入 `.js`，保存并刷新。
2. 打开“UFI-Gate 网闸”，检测环境，核对热点接口和网关（中兴设备桥接口为 `br0`，网关 `192.168.0.1`），安装服务。
3. 打开独立后台（默认 `http://192.168.0.1:8081`），初始化管理员账号，配置认证方式、上网账号、配额与推送。
4. 保留 Root Shell 或 ADB 恢复通道，在 UFI-Tools 宿主 WiFi 管理中将热点设为 OPEN/NONE。
5. 用第二台未受信任的设备完成真机验收，再开放给访客。

> 提示：新连入的设备若没自动弹出认证页，多半是系统把该网络记成了“已验证”。在手机上「忽略此网络」后重新连接即可触发门户探测。

所有插件合计受宿主 5 MiB 存储限制；每个安装包独立校验该上限，请勿同时导入两种架构。完整步骤、配置口径和故障处理见 [安装与管理指南](docs/INSTALL.md)。

## 端口占用

| 端口 | 用途 |
| --- | --- |
| `8090` | 访客认证门户（HTTP 探测与跳转落点） |
| `8081` | 管理员后台（仅管理 MAC 可达） |
| `5354` | 本地 DNS 劫持应答 |
| `67` | DHCP |

认证门户刻意选在 **8090**，避开中兴等设备官方后台占用的 `8080`——若复用 8080，Go 进程抢不到端口会整体退出，连带 8081 管理后台无法访问（`ERR_CONNECTION_REFUSED`）。

## 功能

| 模块 | 能力 |
| --- | --- |
| 管理后台 | 独立管理员登录、设备看板、实时速率与用量排行、日志、浅色与深色主题 |
| 设备管理 | MAC / IP 识别、备注、放行、撤销、封禁、永久信任、管理员解禁 |
| 远程踢人 | 后台「清除会话 / 封禁」时经 hostapd 控制口发 DEAUTH，设备断开重连并自动重新弹认证页 |
| 流量与配额 | 上下行统计、每日 / 历史累计 / 会话 / 固定周期配额、提前限速、满额限速或断网 |
| 门户认证 | 上网账号密码、本地图形验证码、hCaptcha / Turnstile / 自定义验证接口、管理员审批 |
| 审批推送 | 企业微信、飞书、通用 Webhook、SMTP 邮件；带有效期的审批链接 |
| 网络准入 | 未认证 IPv4 默认拒绝、DNS 门户应答、HTTP 跳转；开启期间阻断热点 IPv6 |
| 客户端隔离 | 认证开启后自动 DROP 热点网段内互访，访客之间、访客与管理设备之间三层隔离，等同 AP 隔离 / VLAN 隔离效果；访问网关后台与外网不受影响，白名单内网服务器仍可达 |
| 门户外观 | 网站 Logo 与认证页 Logo 可在后台上传替换，支持自定义品牌图标 |
| 访问策略 | 域名 / IPv4 黑白名单、白名单免认证、警告页与联系信息定制 |
| 生命周期 | 独立常驻、开机启动、停止 / 卸载 / 崩溃恢复、宿主停用检测 |

## 使用边界

- **HTTPS / HSTS 无法无证书警告地透明跳转。** 未认证的 HTTPS、QUIC 与 APP 外网连接会被阻断；认证页由系统“登录网络”、HTTP 请求或本地入口打开。
- 域名过滤基于 DNS 与 IPv4 地址，属于尽力过滤。已认证设备的加密 DNS、VPN、缓存 IP 与共享 CDN 会影响准确性；本项目不提供应用层审计。
- 厂商硬件 NAT / 加速、随机 MAC、固件内核模块必须在实际设备验证；当前仅支持一套热点接口和 IPv4 上网。
- 客户端隔离基于 iptables FORWARD DROP 实现，不依赖宿主是否暴露 AP 隔离开关；服务停止时规则自动清理，不污染系统。
- 流量每 5 秒采样，包含 IP 开销与重传，可能超过配额一个采样窗口；不是运营商计费账本。限速通过字节型 hashlimit 丢包实现。
- 远程审批地址需经 VPN 或 HTTPS 隧道可达；机器人 Webhook 或邮件不会自动让内网设备暴露到公网。
- 远程踢人依赖 hostapd 控制 socket；在没有 hostapd_cli 的设备上由守护进程直连 unix socket，找不到控制口时仅记日志、不影响后台操作。
- 停止或恢复后由本插件恢复网络准入规则；热点的 SSID、加密方式由你在 UFI-Tools 宿主 WiFi 管理中维护，插件不再代为改写或回滚。

这些限制及第三方服务配置详见 [安装与管理指南](docs/INSTALL.md#实现与边界)。

## 紧急恢复

在 UFI Root Shell 或 ADB / su 中执行：

```sh
sh /data/ufi-gate/service.sh recover
```

如服务二进制不可执行，可使用仓库内 [emergency-recover.sh](emergency-recover.sh)。恢复操作只移除本插件的 `UFIG_` 链和接入，不清空全局防火墙。建议首次启用前准备恢复通道。

## 开发与构建

Go 1.24 或更新版本、Python 3；Go 依赖已 vendored。

```sh
go test -mod=vendor -race ./...
go vet -mod=vendor ./...
python3 build.py --arch all --go /path/to/go
```

默认同时构建 ARM32 与 ARM64；可通过 `--arch arm32` 或 `--arch arm64` 单独构建。

输出：单文件 `.js` 安装器、静态 ARM32 / ARM64 二进制、构建信息与 SHA256SUMS。Go 1.24.2 是本次发布使用的工具链。

本地 UI 演示不操作防火墙：

```sh
go build -mod=vendor -buildvcs=false -o ufi-gate-demo ./
./ufi-gate-demo --demo --dir ./demo-state
```

浏览器集成测试需安装 Playwright 及 Chromium，并先生成 `ufi-gate-demo`：

```sh
npm install --no-save playwright
npx playwright install chromium
node tests/ui-test.cjs
```

可通过 `CHROME_PATH` 指定已有浏览器，通过 `PLUGIN_PATH` 指定构建的安装器以验证内嵌负载（ARM32 包同时设置 `PLUGIN_ARCH=arm32`）。上述演示及规则执行器测试不能替代真机验收。

## 版本记录

| 版本 | 说明 |
| --- | |
| `1.0.4` | 门户外观自定义：新增网站 Logo 与认证页 Logo 设置，可以用自己的图片替换默认图标。[详情](docs/releases/v1.0.4.md) |
| `1.0.3` | 插件安装器移除"热点设为 OPEN / 恢复热点配置"按钮，热点参数统一交回 UFI-Tools 原生 WiFi 管理；按钮分组重排。[详情](docs/releases/v1.0.3.md) |
| `1.0.2` | 认证门户 8080→8090 避开中兴官方后台占用；新增 hostapd DEAUTH 远程踢人。[详情](docs/releases/v1.0.2.md) |
| `1.0.1` | 首版：网页认证、设备准入、流量配额、审批推送。[详情](docs/releases/v1.0.1.md) |

GitHub Actions 对推送和 PR 运行逻辑与语法检查。

## 目录

| 路径 | 内容 |
| --- | --- |
| `main.go` | 服务 API、账户、认证、配额、审批、推送与远程踢人 |
| `firewall.go` / `dns.go` | IPv4 准入、计数、限速、DNS 与域名规则 |
| `web/` | 管理后台和访客认证页 |
| `plugin.template.js` | UFI-TOOLS 插件与内嵌服务安装器模板 |
| `service.sh` | 常驻、开机接入、停止 / 卸载 |
| `tests/` / `main_test.go` | 浏览器和逻辑测试 |
| `docs/` | 安装指南、测试记录与版本说明 |
| `scripts/` | Release 打包与校验 |

## 联系

有问题、建议或真机兼容反馈，欢迎通过 Telegram 联系：[@nanbeiluduo](https://t.me/nanbeiluduo)

## 许可与致谢

原创代码按 [MIT License](LICENSE) 提供。`vendor/` 内的 Go 密码库按其原有许可证分发。感谢 [UFI-TOOLS](https://github.com/kanoqwq/UFI-TOOLS) 提供宿主接口；本项目是独立插件，与上游不存在官方隶属关系。
