# 14 — 匿名遥测系统实施记录

完成内容：新增匿名遥测本地设置、首次启动授权弹窗（默认勾选）、随机 UUID v4 安装标识、固定事件白名单、异步短超时发送，以及 About 页面中的遥测开关与 GitHub Releases 更新检查。客户端仅向 `https://telemetry.nfcx.tools/api/telemetry` 发送安装标识、版本、粗粒度系统类型、事件名和时间；不会发送 UID、密钥、dump、卡片内容、读卡器信息、文件路径或日志。

新增 `telemetry-worker/`：包含 Cloudflare Worker、D1 schema、严格请求校验和 secret 保护的聚合 Dashboard。Worker 只选取允许字段写入 D1，不持久化 raw request、headers 或 IP。

遇到的问题：现有仓库没有通用用户偏好配置层，因此使用 `$XDG_CONFIG_HOME` / 平台用户配置目录下的 `NFCX/telemetry.json`，在支持 POSIX mode 的平台以 `0600` 创建。Windows 的 `os.FileMode` 不表达 POSIX owner/group/other 权限，Go 会把以 `0600` 创建的普通文件读回为 `0666`；测试因此在 Windows 验证文件类型和可访问性，仅在 Unix 平台严格断言 `0600`。Wails 生成的前端绑定在本次源代码中同步更新；正式 Wails build 会重新生成它们。

阻塞或未完成事项：Cloudflare D1 数据库 ID、Worker 部署和 Dashboard secret 需要在实际 Cloudflare 账户中设置，未写入仓库。无其他阻塞。

后续修正：About 中的网站与 GitHub 改为调用 Wails 的系统浏览器打开接口，避免桌面 WebView 对普通 HTML 链接的限制。
