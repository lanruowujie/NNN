# NFCX  ：匿名遥测系统

## 目标

为 NFCX 增加一个简单、匿名、隐私优先的遥测系统。

主要目的：

- 了解 NFCX 有多少实际 Installation
- 了解用户使用 Windows / macOS / Linux 的比例
- 了解 NFCX 版本分布
- 了解哪些 NFCX 功能最常被使用
- 了解应用的基本活跃情况

系统分为：

1. NFCX 客户端
2. Cloudflare Worker
3. Cloudflare D1
4. 简单的管理员 Dashboard

遥测不得采集 NFC 卡敏感数据和个人身份信息。

---

# 1. 首次启动授权

NFCX 第一次启动时，如果本地不存在遥测设置，需要显示一个遥测授权弹窗。

内容大意：

帮助改进 NFCX。

允许 NFCX 发送匿名使用统计，例如：

- NFCX 版本
- 操作系统类型
- 使用了哪些 NFCX 功能

明确告诉用户不会发送：

- NFC 卡 UID
- Key
- Dump
- 卡片内容
- 用户个人信息

提供 Checkbox：

[x] 允许发送匿名遥测数据

默认：

勾选。

用户点击确认/继续后保存选择。

以后启动不再重复询问。

---

# 2. Installation ID

如果用户允许遥测，则创建随机：

UUID v4

作为：

installation_id

例如：

550e8400-e29b-41d4-a716-446655440000

该 ID 用于区分不同 NFCX Installation。

必须随机生成。

禁止通过以下信息生成：

- Machine ID
- MAC Address
- Hardware UUID
- Disk serial
- CPU serial
- Username
- Hostname
- NFC Reader serial
- NFC Card UID

installation_id 本身不应该能够反推出设备身份。

生成后保存在 NFCX 本地配置。

同一个 NFCX Installation 后续继续使用同一个 installation_id。

如果用户关闭后重新开启遥测，可以继续使用之前的 installation_id。

---

# 3. 本地配置

在现有 NFCX 配置系统中增加类似：

telemetry_enabled
installation_id

例如：

{
  "telemetry_enabled": true,
  "installation_id": "..."
}

不要为了遥测单独建立复杂配置系统，应复用当前项目配置方式。

---

# 4. About 页面

在 NFCX 主 UI 增加：

About / 关于

按钮或菜单入口。

打开后显示 About Dialog。

至少包含：

NFCX

Version:
当前版本号

Author:
项目作者

Website:
https://nfcx.tools

GitHub:
项目 GitHub 地址

以及：

[ Check for Updates ]

Anonymous Telemetry
[ ON / OFF ]

About 页面必须接入 NFCX i18n。

---

# 5. 遥测开关

用户可以在 About 页面随时：

开启 / 关闭匿名遥测。

关闭后：

- 立即停止发送新的 telemetry event
- 不再创建新的 telemetry event queue
- NFCX 其他功能完全不受影响

可以保留本地 installation_id。

重新开启时继续使用原 installation_id。

---

# 6. 遥测数据范围

允许发送的数据必须采用白名单。

基础字段：

installation_id
timestamp
version
os
event

例如：

{
  "installation_id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": "2026-09-14T12:00:00Z",
  "version": "1.0.0",
  "os": "windows",
  "event": "card_scan_clicked"
}

操作系统只保存粗粒度类型：

windows
macos
linux
other

不要发送完整 OS build、设备型号等信息，除非未来明确扩展 Spec。

---

# 7. Event 设计

遥测主要记录用户使用了 NFCX 的哪些功能。

根据当前实际 UI 和功能建立事件白名单。

例如：

app_started
reader_scan_clicked
card_scan_clicked
read_clicked
write_clicked
dump_clicked
key_recovery_clicked
settings_opened
about_opened
update_check_clicked

实际 Event 名称需要根据现有功能整理。

不要简单地对所有 GUI Button 自动埋点。

必须建立明确的：

TELEMETRY_EVENT_ALLOWLIST

或等价机制。

只有明确允许的事件才能发送。

不要把按钮文字直接作为 Event。

这样可以防止未来某个按钮包含动态内容时被意外上传。

---

# 8. 严禁采集的数据

客户端遥测严禁主动采集或发送：

- NFC UID
- Card Serial Number
- Card contents
- NFC Dump
- Key A
- Key B
- 破解/恢复得到的 Key
- NFC protocol raw payload
- Reader serial number
- Reader hardware ID
- 用户打开的文件路径
- 文件名
- Username
- Hostname / Computer Name
- Email
- MAC Address
- Machine ID
- Hardware UUID
- Disk serial
- Clipboard
- 任意自由文本
- 任意未经白名单允许的日志
- Exception 中可能包含的敏感上下文
- IP Address

遥测 API 不允许发送任意 properties 字典来规避这个限制。

如果 Event 未来确实需要额外属性，必须单独把字段加入 schema 和白名单。

---

# 9. 客户端发送机制

Telemetry 必须是：

Best Effort

遥测永远不能影响 NFCX 正常功能。

要求：

- 异步发送
- 设置较短网络 Timeout
- 网络错误直接忽略或写 Debug Log
- 不弹出错误窗口
- 不阻塞 NFCX 启动
- 不阻塞 Reader Scan
- 不阻塞 Card Scan
- 不阻塞 Read
- 不阻塞 Write
- 不阻塞 Dump
- 不阻塞 Key Recovery

禁止因为 Telemetry Server 不可用导致 NFCX 卡顿。

不要建立无限 Retry。

 
如果发送失败，允许直接丢弃 Event。

---

# 10. Telemetry Endpoint

Telemetry Server 使用：

Cloudflare Worker + Cloudflare D1

提供：

POST /api/telemetry

客户端发送 JSON。

Worker 验证请求。

验证至少包括：

- Content-Type
- 请求大小
- installation_id 是否为合法 UUID
- version 长度是否合理
- os 是否在白名单
- event 是否在白名单
- JSON schema 是否符合预期

非法请求返回：

400

合法请求写入 D1 后：

204 No Content

---

# 11. 服务端隐私保护

即使未来客户端出现 Bug，服务端也必须执行第二层数据最小化。

Worker 只读取明确允许的字段：

installation_id
timestamp
version
os
event

不要把整个 Request JSON 直接写入数据库。

对于额外字段：

直接忽略或拒绝请求。

不要保存 Raw Request Body。

不要保存 Headers。

尤其不要将：

CF-Connecting-IP

或其他来源 IP 写入 D1。

应用代码不要主动记录来源 IP。

如果 Cloudflare 平台自身存在基础网络日志，这是基础设施行为，不应将其进一步复制进 NFCX 遥测数据库。

---

# 12. D1 Schema

创建 D1 Database。

例如表：

telemetry_events

字段：

id INTEGER PRIMARY KEY AUTOINCREMENT

installation_id TEXT NOT NULL

event TEXT NOT NULL

app_version TEXT

os TEXT

created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP

可以根据实际查询增加 Index：

installation_id
event
created_at
app_version
os

不要存储 IP。

不要存储 User-Agent。

不要存储其他设备 fingerprint。

---

# 13. Dashboard

Worker 同时提供简单管理员页面：

GET /dashboard?secret=xxxxxx

用于查看 NFCX 遥测统计。

这是个人/小型开源项目，因此初版不需要复杂登录系统。

---

# 14. Dashboard Secret

Secret 不允许写死在 Git Repository。

使用：

Cloudflare Worker Secret

例如：

TELEMETRY_DASHBOARD_SECRET

访问：

/dashboard?secret=<secret>

Worker 比较 URL 中 secret 与环境 Secret。

如果：

- Secret 缺失
- Secret 错误

返回：

403 Forbidden

Dashboard 页面及 API 必须进行同样授权。

不要在 HTML 中暴露真正的 Secret。

---

# 15. Dashboard 数据

Dashboard 主要显示聚合数据。

至少包括：

## Total Installations

COUNT(DISTINCT installation_id)

---

## Active Installations

最近 30 天至少发送过一个 Event 的 installation_id 数量。

---

## Events Today

当天收到的 Event 数量。

---

## Total Events

所有 Event 数量。

---

## OS Distribution

例如：

Windows 70%
macOS 20%
Linux 10%

---

## NFCX Version Distribution

例如：

1.0.0
1.0.1
1.1.0

对应 Installation 或 Event 分布。

---

## Feature Usage

统计 Event 使用次数。

例如：

Card Scan
Read
Write
Dump
Key Recovery

---

## Usage Over Time

至少提供按天统计：

Daily Events

最好同时可以显示：

Daily Active Installations

---

# 16. Dashboard UI

Dashboard 做得稍微漂亮一些，但保持简单。

可以使用：

HTML
CSS
少量 JavaScript

允许使用非常轻量的图表库，也可以直接使用 CSS/Canvas/SVG。

不要为了这个 Dashboard 引入复杂 SPA Framework。

Dashboard 应至少具有：

- Summary Cards
- 简单图表
- Feature Usage 排名
- Version Distribution
- OS Distribution

Desktop 优先，同时保证手机打开不会完全错位。

---

# 17. Installation ID 展示

Dashboard 默认只展示聚合统计。

不要默认提供：

Installation ID 列表

因为正常分析不需要查看具体 Installation。

如果实现 Debug 页面，也不要展示任何 PII，因为系统本身不应该存在 PII。

---

# 18. Check for Updates

About 页面提供：

Check for Updates

检查当前 NFCX Version 是否为最新版。

优先利用 GitHub Releases 获取 Latest Release。

如果检查失败：

显示简单提示即可。

Update Check 与 Telemetry 是两个独立功能。

即使：

telemetry_enabled = false

用户主动点击：

Check for Updates

仍然可以正常检查更新。

不要把 Update Check 偷偷作为 Telemetry Event 之外的数据采集渠道。

如果 Telemetry 开启，可以发送：

update_check_clicked

事件。

---

# 19. i18n

以下新增 UI 必须从一开始接入现有 i18n：

- 首次 Telemetry Consent
- About
- Telemetry Toggle
- Check for Updates
- Update Result
- Telemetry 相关错误/说明

支持：

English
简体中文

禁止在新增 UI 中大量硬编码英文字符串。

---

# 20. 安全与鲁棒性

Telemetry API 必须限制：

- Request body size
- JSON 类型
- 字段长度
- Event 白名单

由于公开客户端无法安全保存 API Secret，因此：

POST /api/telemetry

不要求在客户端内置秘密 API Key。

不要把所谓 Secret 编译进客户端假装安全。

服务端依赖：

- 严格 Schema
- 白名单
- Payload Limit
- Cloudflare 自身基础防护

防止明显滥用。

Dashboard 则必须通过 Server-side Secret 保护。

---

# 21. 不在本 Spec 范围内

初版不需要：

- 用户账户
- OAuth
- Cloudflare Access
- 登录页面
- Machine Fingerprinting
- Crash Reporting
- 完整日志上传
- NFC Dump 上传
- 在线用户追踪
- Geographic Analytics
- IP Analytics
- Persistent Offline Event Queue
- 实时 WebSocket Dashboard
- Google Analytics
- 第三方 Telemetry SaaS

---

# 22. 验收标准

完成后必须满足：

- 第一次启动出现 Telemetry Consent
- 默认勾选允许
- 用户确认后保存设置
- 后续启动不重复询问
- 使用随机 UUID v4 installation_id
- installation_id 不依赖任何硬件信息
- About 页面存在
- About 显示 NFCX Version
- About 显示 Author
- About 显示 nfcx.tools
- About 可以 Check for Updates
- About 可以开启/关闭 Telemetry
- Telemetry 关闭后立即停止发送
- Event 使用明确白名单
- 不上传 UID
- 不上传 Key A / Key B
- 不上传 Dump
- 不上传 Card Content
- 不上传 Username
- 不上传 Hostname
- 不上传 Machine ID
- 不上传 MAC
- 不上传 Reader Serial
- 不上传 IP
- Worker 不将 IP 写入 D1
- Worker 不保存 Raw Request
- Telemetry 网络失败不会影响 NFCX
- POST /api/telemetry 正常工作
- 数据正确写入 D1
- Dashboard 可以查看聚合统计
- Dashboard 必须使用 secret 授权
- Secret 不存在 Git Repository
- Dashboard 能显示 Installation / OS / Version / Feature / Usage 趋势
- 所有新增客户端 UI 支持 English / 简体中文