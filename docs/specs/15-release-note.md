# NFCX Spec 1：README、文档与官方网站

## 目标

重新整理 NFCX 的公开文档，并创建 NFCX 官方网站 `nfcx.tools`。

当前 README 偏向开发手册，需要改成面向普通用户的项目介绍，让第一次看到 NFCX 的用户可以快速理解：

- NFCX 是什么
- NFCX 可以做什么
- 支持哪些 NFC 读卡器
- 是否需要安装驱动
- 如何下载和使用
- 项目的 License 和第三方依赖情况

同时创建一个轻量、现代、响应式的官方网站，最终部署到 Cloudflare Pages。

---

# 1. README 重构

## 1.1 README 定位

README 不再作为详细开发手册。

README 应面向 NFCX 普通用户，内容简洁清晰，不需要介绍内部代码结构、类设计、协议实现等开发细节。

README 应包含以下主要内容：

1. NFCX 简介
2. 主要功能
3. 支持的读卡器
4. 驱动和运行要求
5. 下载方式
6. 简要使用方法
7. 软件截图
8. 隐私与匿名遥测说明
9. License
10. 官网和项目链接

不要在 README 中宣传当前尚未实现的功能。

---

# 2. README i18n

README 只需要支持：

- English
- 简体中文

默认 README 为英文：

README.md

中文版：

README.zh-CN.md

两个 README 顶部都提供语言切换，例如：

English | 简体中文

README.md 为 GitHub 默认显示版本。

两个版本结构和信息应基本一致。

---

# 3. README 内容

## 3.1 项目简介

README 开头提供：

- NFCX 名称
- 一句话介绍
- 项目 Logo（如果当前没有 Logo，可以预留位置）
- 主界面截图
- 官方网站：https://nfcx.tools
- GitHub Release 下载入口

项目描述应面向用户，不要写成技术架构介绍。

---

# 4. 功能介绍

根据当前代码中实际已经实现的功能整理 Feature 列表。

例如可能包括：

- NFC 读卡器检测
- NFC 卡片扫描
- 卡片基础信息读取
- MIFARE Classic 操作
- Key A / Key B 相关功能
- Dump
- Read
- Write
- Key recovery / cracking
- 其他当前已经实现的 NFC 工具

必须检查当前代码后再确定最终列表。

禁止宣传实际上没有实现的功能。

---

# 5. 支持的读卡器

README 中增加 Supported Readers。

重点检查并说明：

- PN532
- ACR122U
- ACR1552U
- 其他当前代码理论支持或实际支持的 libnfc 兼容设备

对于每类读卡器说明：

- 连接方式，例如 USB / Serial
- 是否需要驱动
- 使用的后端，例如 serial / libnfc
- Windows / macOS / Linux 的兼容情况
- 是否实际测试过

必须区分：

- Tested
- Expected / Compatible
- Unsupported

没有实际测试过的硬件不要标记为 Tested。

---

# 6. 驱动说明

README 应简单说明用户使用不同读卡器前需要安装什么。

例如：

- PN532 是否需要额外驱动
- ACR122U 的 PC/SC / libnfc 相关要求
- ACR1552U 的驱动要求
- Windows 特殊驱动要求
- macOS / Linux 的依赖要求

保持简洁。

详细开发环境配置不需要放进主 README。

---

# 7. 下载和安装

提供非常简单的用户流程：

1. 打开 GitHub Releases
2. 下载对应操作系统版本
3. 解压或安装
4. 如有必要安装读卡器驱动
5. 插入 NFC 读卡器
6. 启动 NFCX


---

# 8. Quick Start

提供一个简短的使用流程。

例如：

1. 连接 NFC 读卡器
2. 启动 NFCX
3. 选择或扫描 Reader
4. 将 NFC 卡放在读卡器上
5. Scan Card
6. 查看卡片信息
7. 根据需要使用 Read / Write / Dump / Key Recovery 等功能

Quick Start 不需要变成完整操作手册。

---

# 9. 软件截图

创建统一的图片目录，例如：

docs/images/

预留：

docs/images/main-window.png
docs/images/card-scan.png
docs/images/mifare-tools.png
docs/images/about.png

README 中预留对应图片位置。

图片由项目维护者后续自行截图替换，因此不需要生成虚假 UI 截图。

如果图片暂时不存在，应确保 README 不因为错误引用而变得混乱，可以使用明确 TODO 或 placeholder。

---

# 10. 隐私与遥测说明

README 简单介绍 NFCX 的匿名遥测功能。

需要明确：

- 遥测是可选的
- 用户首次启动时可以选择
- 用户之后可以随时关闭
- 只用于了解 NFCX 的功能使用情况、版本和操作系统分布
- 不采集 NFC 卡 UID
- 不采集 Key A / Key B
- 不采集 Dump
- 不采集卡片内容
- 不采集用户名
- 不采集设备名
- 不采集 Machine ID
- 不主动采集或保存 IP
- 不采集其他 PII

详细实现以 Telemetry Spec 为准。

---

# 11. License 审计

不要直接假设 NFCX 可以使用 MIT License。

在确定项目 License 前，先检查项目实际使用和分发的第三方组件。

重点检查：

- libnfc
- mfoc
- 其他调用的 NFC utilities
- 随 Release 一起分发的 executable / DLL / library
- 前端/UI dependency
- 项目中复制或修改过的第三方代码

需要区分：

1. Go import / 动态链接
2. 调用外部 executable
3. 直接打包并重新分发第三方 binary
4. 仅要求用户自行安装的工具

输出一个简短的第三方 License 审计结果。

根据审计结果，再决定 NFCX 自己适合使用：

- MIT
- Apache-2.0
- GPL
- 或其他兼容 License

不要未经检查直接选择 License。

如第三方组件要求保留版权或 License Notice，则创建：

THIRD_PARTY_NOTICES.md

最终根据审计结果创建或更新：

LICENSE

README 中增加 License 章节。

---

# 12. nfcx.tools 官方网站

创建 NFCX 官方网站。

域名：

https://nfcx.tools

最终使用 Cloudflare Pages 部署。

网站应优先采用静态：

HTML
CSS
JavaScript

除非当前项目已经存在合理的 Web 构建系统，否则不要为了简单官网引入 React、Vue、Next.js 等大型框架。

建议目录：

website/
  index.html
  zh-CN/
    index.html
  assets/
    css/
    js/
    images/

Cloudflare Pages 应能够直接部署网站目录。

---

# 13. 官网设计

网站定位：

轻量、现代、偏开发工具风格。

不要做成企业官网。

需要响应式支持：

- Desktop
- Tablet
- Mobile

不要加入大量动画或大型 JS dependency。

---

# 14. 官网内容

首页至少包含：

## Hero

显示：

NFCX

简短描述。

提供主要按钮：

Download
GitHub

可以预留主界面 Screenshot。

---

## Features

以 Card / Grid 形式展示主要功能。

内容必须来自当前实际实现的 NFCX 功能。

---

## Supported Readers

展示：

- PN532
- ACR122U
- ACR1552U
- 其他实际支持设备

并标注测试状态。

---

## Screenshots

展示 NFCX 实际 UI。

图片由维护者后续提供，因此先建立结构和 placeholder。

---

## Quick Start

简短介绍：

Connect Reader
→ Launch NFCX
→ Scan Card
→ Use NFC Tools

---

## Privacy

介绍匿名遥测。

强调：

No card UID
No keys
No dumps
No card contents
No personal information

---

## Download

提供 GitHub Releases 入口。

如果目前没有自动获取最新 Release 的机制，可以先使用稳定链接，不要为了这个功能引入复杂后端。

---

## Project

提供：

GitHub
README
License
Third Party Notices（如果存在）

---

## Footer

至少包含：

NFCX
nfcx.tools
GitHub
License
Copyright / Project information

---

# 15. 官网 i18n

官网只支持：

English
简体中文

默认：

/

为英文。

中文版：

/zh-CN/

页面顶部提供：

EN | 中文

不需要复杂的 JavaScript i18n framework。

两个静态页面即可。

---

# 16. 不在本 Spec 范围内

本阶段不需要：

- 用户账号
- 登录系统
- CMS
- Blog
- 在线 NFC 工具
- Cloudflare Worker 后端
- Telemetry Dashboard
- 自动上传 Screenshot
- 多语言自动翻译
- SEO 管理后台

Telemetry Worker 属于独立 Spec。

---

# 17. 验收标准

完成后必须满足：

- README.md 为英文默认 README
- README.zh-CN.md 为简体中文
- 两者可以互相切换
- README 已从开发手册改为用户导向
- 功能介绍与当前实际实现一致
- 支持的 Reader 有清晰说明
- 驱动要求有清晰说明
- 有简短 Quick Start
- 已建立 Screenshot 目录和占位
- 已进行第三方 License 审计
- 已根据审计结果确定 NFCX License
- 必要时存在 THIRD_PARTY_NOTICES.md
- 已创建 nfcx.tools 静态网站
- 网站支持 English / 简体中文
- 默认英文
- 网站可以部署至 Cloudflare Pages
- Desktop / Mobile 显示正常
- 网站没有宣传未实现功能