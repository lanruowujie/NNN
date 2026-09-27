# 规格 01：项目骨架与 GUI 外壳

## 目标

建立 NFCX 的可编译基础工程，固定 Go 1.25.1 和 Wails v2，并创建不依赖真实 NFC 硬件的桌面 GUI 外壳。该阶段结束时，开发者应能启动应用、浏览主要页面并使用模拟状态验证前后端通信。

## 前置条件

- 本机 `go version` 为 `go1.25.1`。
- 已安装满足 Wails v2 要求的平台构建依赖。
- 不要求安装 libnfc。

## 范围内

- 初始化 `go.mod`，module 名称统一且可长期使用。
- 初始化 Wails v2，前端使用 Vanilla TypeScript。
- 建立 `cmd/nfcx`、`app`、`frontend` 和主要 `internal` 目录。
- 定义 GUI 与 Go 后端之间的基础 DTO。
- 创建主窗口布局和模拟数据服务。
- 建立最小的单元测试、格式化和 CI 检查入口。

## 不在本阶段实现  

- libnfc、CGO 或真实设备连接；
- MIFARE 协议；
- dump 文件读写；
- 破解引擎；
- 安装包签名。

## 实现要求

`go.mod` 应声明 Go 1.25.1。Wails binding 只暴露面向 GUI 的 application service，不要把未来的 Reader、C 指针或内部错误直接暴露给前端。

主界面至少预留以下区域：

- 设备选择和连接状态；
- 卡片 UID、ATQA、SAK 和卡型摘要；
- 扇区/block 数据工作区；
- 密钥区域；
- 操作按钮区域；
- 任务进度和日志区域。

本阶段使用 mock service 返回固定设备和卡片。前端必须能触发一个模拟长任务、接收进度事件并取消它，以提前确定异步模型。

建议的初始包结构：

```text
cmd/nfcx/
app/
frontend/
internal/nfc/
internal/device/
internal/mifare/
internal/attack/
internal/workflow/
testdata/
```

不要为了填满页面而引入大型 UI 组件库。优先使用 CSS variables 定义颜色、间距和状态色。

## 自动化测试

- application service 返回的 DTO 能正确 JSON 序列化；
- 模拟任务会发出开始、进度、完成事件；
- 取消模拟任务后不会继续发送完成事件；
- `go test ./...` 不要求 Wails 窗口或真实硬件。

## 验收标准

- [ ] `go.mod` 明确使用 Go 1.25.1。
- [ ] 开发模式下可以启动 Wails 窗口。
- [ ] 生产构建至少能在当前开发平台完成。
- [ ] GUI 能显示模拟设备和模拟卡片。
- [ ] GUI 能开始并取消模拟任务，窗口全程可交互。
- [ ] 前端没有直接依赖 libnfc 或平台命令。
- [ ] `go test ./...` 通过。

## 验收证据

保留启动截图、生产构建命令输出和测试输出。该阶段不需要真实读卡器演示。

