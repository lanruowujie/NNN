# 13：客户端国际化

## 完成内容

- 新增 `frontend/src/locales/en.json` 与 `frontend/src/locales/zh-CN.json`，以稳定语义 key 提供英文和简体中文资源。
- 新增统一的 `t()` 翻译 API，支持 `{name}` 形式的变量插值、英文回退以及最终显示 key 的安全回退。
- 首次启动时读取浏览器/系统 Locale：`zh-CN`、`zh-SG`、`zh-Hans` 及裸 `zh` 选择简体中文；其他 Locale 或检测失败选择英文。
- 复用前端已有的 `localStorage` 偏好保存方式，将手动语言选择保存在 `nfcx-language`；已保存选择始终优先于系统 Locale。
- 在顶栏加入语言选择器。选择后静态界面和当前的主要动态视图会立即重绘，无需重启。
- 将主窗口、核心工作区、密钥库、写卡预检、恢复和 UID 对话框等现有静态 UI 文本迁入资源文件。
- Vite 将 JSON 资源直接打入前端 bundle，Windows、macOS 和 Linux 的 Wails 产物通过同一 `frontend/dist` 嵌入资源路径携带翻译资源。
- 卡片识别结果现在直接显示卡型名称（例如 `MIFARE Classic 1K`），不再在 UI 中加入“推测为”前缀；DTO 的 `Inferred` 标记仍保留，供程序继续区分保守识别结果。

## 验证

- 运行 `npm run build`（在 `frontend/`）通过 TypeScript 检查和 Vite 生产构建。

## 遇到的问题

无。

## 阻塞或未完成事项

- 后端仍会向前端发送若干中文的设备状态和任务技术详情。这些信息保留为可调试的原始详情；后续应将它们逐步改为稳定错误/事件码，并由前端使用同一翻译层渲染用户提示。
