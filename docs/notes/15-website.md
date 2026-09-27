# 15 — NFCX 官方网站实施记录

完成内容：新增 `website/` 下的静态、中英双语、响应式 NFCX 官网。`/` 为英文，`/zh-CN/` 为简体中文；页面使用共享 CSS、无框架 JavaScript 依赖和本地品牌资源，可直接将 `website/` 作为 Cloudflare Pages 输出目录。页面仅说明现有的 PN532 UART 与 MIFARE Classic 工作流：设备与卡片发现、读取/编辑/恢复、密钥恢复和写入安全保护；下载与 GitHub 链接均指向项目正式地址。

同时将 README 重构为面向用户的英文默认版与简体中文版，新增 `LICENSE`、`THIRD_PARTY_NOTICES.md` 和 `docs/images/` 截图预留说明。许可证审计确认：NFCX 动态链接 LGPL-3.0-or-later 的 libnfc；mfoc、mfcuk、mfoc-hardnested 作为独立 GPL-2.0-or-later 外部程序分发；`nfc-mfsetuid` 为 BSD-2-Clause。因此 NFCX 自身源码采用 MIT，同时在发行运行时中保留各组件的许可证、来源锁定和补丁。

品牌资源更新：采用深海军蓝标签与青色 NFC 波纹标志，生成并替换 `build/appicon.png`；同一图形用于官网 logo、32px favicon、Apple touch icon、README 和 Wails 前端顶部品牌区。图标为带透明背景的 PNG，适用于 Wails 现有跨平台打包流程：macOS 与 Windows 由 Wails 从 `build/appicon.png` 生成原生资源，Linux AppImage 打包脚本将其安装为 `NFCX.png`。

`make dev` 现在传入 Wails 的 `-forcebuild`。Wails 的开发 app bundle 会把 `build/appicon.png` 转成 `build/bin/NFCX.app/Contents/Resources/iconfile.icns`；该开关确保更换 PNG 后的下一次开发启动不复用旧 bundle 图标。

网站入口位于 `website/index.html`，可直接作为 Cloudflare Pages 的静态输出；部署前可先通过任意本地静态文件服务器检查版式。

遇到的问题：无。

阻塞或未完成事项：官网已使用 `docs/images/main.jpg` 的副本作为首张真实应用截图；维护者仍可补充更多截图，并需要在 Cloudflare 账户中绑定 `nfcx.tools` DNS/Pages 自定义域名。无其他阻塞。
