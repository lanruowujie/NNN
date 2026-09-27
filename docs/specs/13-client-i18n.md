# NFCX ：应用程序 i18n

## 目标

为 NFCX Desktop Application 增加完整的基础国际化支持。

初始只支持：

- English
- 简体中文

默认语言根据用户操作系统语言自动决定。

用户可以在 NFCX 中手动修改语言，并持久化保存。

当前阶段不需要支持第三种语言，但架构上不要把语言判断硬编码成大量 if/else，以便未来可以方便扩展。

---

# 1. 支持语言

初版支持：

en
zh-CN

对应显示名称：

English
简体中文

英文同时作为：

Fallback Language

---

# 2. 第一次启动语言选择

如果本地没有保存过用户语言设置，则读取操作系统 Locale。

规则：

简体中文相关系统 Locale：

使用 zh-CN。

其他 Locale：

使用 en。

例如：

zh-CN -> zh-CN
zh-SG -> zh-CN

en-US -> en
en-GB -> en
ja-JP -> en
ko-KR -> en
de-DE -> en

对于其他中文 Locale，需要根据实际 Locale 信息合理处理。

目标是：

明显属于简体中文用户环境 -> zh-CN

无法确定 -> en

如果系统 Locale 获取失败：

使用 en。

语言检测失败绝对不能影响 NFCX 启动。

---

# 3. 用户语言优先级

语言选择优先级：

1. 用户之前手动保存的语言
2. 操作系统 Locale
3. English

也就是说：

用户一旦手动选择语言，以后启动 NFCX 都使用该语言。

即使操作系统语言发生变化，也不要覆盖用户选择。

---

# 4. Translation Resources

所有用户可见 UI 文本从代码中抽离。

建议：

locales/
  en.json
  zh-CN.json

也可以根据现有项目技术栈选择更合适的资源格式，但必须保持独立语言资源文件。

例如：

en.json

{
  "app.title": "NFCX",
  "common.ok": "OK",
  "common.cancel": "Cancel",
  "common.confirm": "Confirm",
  "reader.scan": "Scan Readers",
  "card.scan": "Scan Card",
  "about.title": "About NFCX"
}

zh-CN.json

{
  "app.title": "NFCX",
  "common.ok": "确定",
  "common.cancel": "取消",
  "common.confirm": "确认",
  "reader.scan": "扫描读卡器",
  "card.scan": "扫描卡片",
  "about.title": "关于 NFCX"
}

---

# 5. Translation Key 规范

Translation Key 使用稳定的语义名称。

例如：

common.ok
common.cancel

reader.scan
reader.not_found
reader.connected

card.scan
card.read
card.write

mifare.dump
mifare.key_a
mifare.key_b

settings.language

about.title
about.version
about.website

telemetry.title
telemetry.description
telemetry.enabled

不要使用完整英文句子作为 Key。

不要使用：

"Scan Card": "扫描卡片"

这种结构。

使用：

"card.scan": "扫描卡片"

方便未来修改英文文本而不影响 Key。

---

# 6. Translation API

提供统一的 Translation API。

例如概念上：

t("card.scan")

UI 代码只调用 Translation Layer。

禁止在 UI 中到处出现：

if language == "zh-CN":
    text = "扫描"
else:
    text = "Scan"

语言判断必须集中在 i18n 模块中。

---

# 7. Existing UI Migration

检查当前 NFCX 所有用户可见文本。

迁移至少包括：

- Main Window
- Button
- Menu
- Tabs
- Labels
- Status Bar
- Dialog
- Confirmation
- Error Message
- Success Message
- Warning
- Tooltip
- Reader UI
- Card UI
- MIFARE UI
- Read / Write UI
- Dump UI
- Key Recovery UI
- Settings

不要只翻译主界面按钮。

---

# 8. 不应该翻译的内容

以下技术内容通常保持原样：

NFCX

PN532
ACR122U
ACR1552U

MIFARE Classic

UID

Key A
Key B

SAK
ATQA

ISO 14443
ISO-DEP

Hexadecimal Data

Card UID

Key Value

Dump Raw Data

Reader Model

技术协议名称不要为了中文化强行翻译。

---

# 9. 动态字符串

i18n 系统必须支持变量插值。

例如：

reader.found

English:

"{count} reader(s) found"

中文：

"发现 {count} 个读卡器"

代码：

t("reader.found", count=2)

不要通过字符串拼接构建翻译文本。

例如不要：

"Found " + count + " readers"

否则中文语序难以处理。

---

# 10. Language Selector

在 NFCX UI 中提供语言切换入口。

简单的一个按钮切换语言

显示：

Language

English
简体中文

---

# 11. Language Persistence

用户手动选择后保存到 NFCX 本地配置。


应复用现有配置机制。

不要为了 i18n 单独建立另一套配置存储。

---

# 12. Runtime Language Switching

优先实现：

修改语言后 UI 立即更新。

如果当前 GUI Framework 实现 Runtime Refresh 会导致大量不必要的架构修改，则初版允许：

修改语言
→ 保存设置
→ 提示 Restart NFCX
→ 重启后应用新语言

不要为了实现无重启切换而破坏现有 UI 架构。

如果可以低成本实现 Runtime Switching，则优先实现。

---

# 13. Fallback

任何 Translation 缺失都不能导致 NFCX Crash。

Fallback 顺序：

当前语言
↓
English
↓
Translation Key

例如：

当前语言：

zh-CN

请求：

t("reader.scan")

如果：

zh-CN.json

不存在该 Key，则使用：

en.json

如果英文也不存在，则显示：

reader.scan

开发环境可以同时输出 Warning：

Missing translation: reader.scan

正式环境不要因为 Translation 缺失弹 Error Dialog。

---


# 14. 新功能必须使用 i18n

完成 i18n 后，后续所有新增用户 UI 必须使用 Translation Layer。

特别是之后的：

Telemetry Consent

About Dialog

Telemetry Toggle

Check for Updates

Update Result

Privacy Description

这些 UI 从一开始就必须同时提供：

English
简体中文

不要先硬编码英文再后续返工。

---

# 15. Error Message

用户可见错误信息应该翻译。

但是：

底层 Exception
Debug Log
libnfc 原始输出
mfoc 原始输出
Command Output

不要求全部翻译。

推荐方式：

用户 UI：

"读取卡片失败。"

技术详情：

"Authentication failed for sector 3"

这样既保持中文体验，又保留调试价值。

---

# 16. Packaging

Windows / macOS / Linux Release 中必须包含 i18n Resource。

不能出现：

开发环境中文正常

但是 Release 因为 locales/*.json 没有被打包导致全部显示 Translation Key。

检查当前：

build scripts
GitHub Actions

并确保 Resource 被正确包含。

---

 

# 17. 不在本 Spec 范围内

初版不需要：

- Traditional Chinese
- Japanese
- Korean
- German
- French

架构应保持可扩展，但不要提前实现这些功能。

---

# 23. 验收标准

完成后必须满足：

- NFCX 支持 English
- NFCX 支持简体中文
- 默认根据 OS Locale 选择语言
- 简体中文环境默认 zh-CN
- 其他环境默认 English
- Locale 获取失败使用 English
- 用户可以手动选择语言
- 用户语言选择会持久化
- 用户选择优先于 OS Locale
- English 为 Fallback
- 所有主要 UI 文本进入 Translation Resource
- UI 不存在大量语言 if/else
- Translation 通过统一 t() 或等价接口获取
- 缺少中文 Translation 时回退 English
- 缺少所有 Translation 时显示 Key
- Translation 缺失不会导致 Crash
- 技术术语不会被不合理翻译
- 动态文本支持参数插值
- 用户可见 Error 支持 i18n
- Debug Log 不要求翻译
- locales 使用 UTF-8
- Windows Release 正确包含 Translation Resource
- macOS/Linux 构建如果存在，也正确包含 Translation Resource
- 新增 Telemetry / About UI 可以直接使用当前 i18n 架构