# 规格 01 实施记录：项目骨架与 GUI 外壳

日期：2026-09-12

## 实际完成内容

- 将 Go module 固定为 `github.com/BennyThink/NFCX`，Go 基线保持 `1.25.1`。
- 引入 Wails v2.15.0，并建立根目录 Wails 构建入口与 `cmd/nfcx` 桌面入口。
- 建立 `app`、`frontend`、`internal/nfc`、`internal/device`、`internal/mifare`、`internal/attack`、`internal/workflow` 和 `testdata` 目录。
- 在 `app` 层定义设备、连接、卡片、block、扇区密钥状态、操作和任务事件 DTO。
- Wails 只绑定 `app.Bindings`；runtime 生命周期、Wails event 和模拟 application service 保持分层。
- 实现固定模拟 PN532/ACR122U 设备、模拟 MIFARE Classic 1K 卡片、16 个 block 和扇区密钥状态。
- 实现可取消的模拟长任务，事件顺序为开始、进度和完成，取消成功后只产生取消终态。
- 使用 Vanilla TypeScript 和 CSS variables 实现完整桌面工作台外壳，没有引入组件库。
- 添加 DTO JSON、任务事件、取消语义和错误类型单元测试。
- 添加 `Makefile` 检查入口和 GitHub Actions 基础检查。
- 完成 `wails dev` 启动验收，Vite watcher、binding 生成和开发窗口均正常。
- 在 macOS arm64 上完成生产构建，并对生产窗口执行模拟任务启动和取消验收。

## 实现过程中遇到的问题

### Wails 对入口目录的静态分析

Wails v2 的 binding 生成命令从当前项目根目录分析 Go main package。只在 `cmd/nfcx` 放置入口时，直接执行 `wails generate module` 和根目录 `wails build` 会报告根目录没有 Go 文件。

最终保留 `cmd/nfcx` 正式目录，同时增加很薄的根目录 Wails 启动器；两者复用 `internal/desktop.Run`，没有复制应用逻辑。这样标准 Wails 命令和 `go run ./cmd/nfcx` 都有明确入口。

### 沙箱中的 macOS 原生链接

受限沙箱不能完整访问 macOS 原生编译工具链，Wails 编译阶段只返回 `exit status 1`。在获准的本机编译环境中运行后，应用成功链接、打包并自签名。Wails 构建过程为 macOS 自动添加 `UniformTypeIdentifiers` framework 和最低系统版本参数。

## 阻塞或未完成事项

无。本阶段明确排除的 libnfc、真实硬件、MIFARE 操作、dump 和攻击引擎留给后续规格。

## 验收证据

见 [`docs/evidence/01-project-scaffold/README.md`](../evidence/01-project-scaffold/README.md)。
