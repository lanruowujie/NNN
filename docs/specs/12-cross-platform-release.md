# 规格 12：跨平台构建与发布

## 目标

建立可复现的 CI/CD，为 Windows、macOS 和 Linux 生成包含 Wails、libnfc 和外部引擎的可运行安装包，并满足动态库加载、签名、许可证和基本 smoke test 要求。

## 前置条件

- 规格 01 至 11 的目标功能已经验收，或明确列出不进入首个发布版本的可选功能。
- 所有第三方版本和许可证已固定。

## 首批目标

- macOS arm64；
- Windows amd64；
- Linux amd64。

其他架构后续增加，不应阻塞首个发布版本。

## 范围

- 为三个目标使用各自的原生 CI runner。
- 固定 Go 1.25.1、Wails、Node 和 C 工具链版本。
- 构建或获取经过校验的 libnfc、libusb/PCSC 依赖和破解引擎。
- 构建 Wails 生产应用。
- 设置正确的 DLL/dylib/so 搜索路径。
- 生成 Windows 安装包或 ZIP、macOS `.app`/DMG、Linux AppImage/tar.gz。
- macOS 嵌套签名、公证和 stapling。
- Windows 代码签名在证书可用时启用；无证书构建必须明确标注。
- 生成 SHA-256、许可证目录和第三方组件清单。
- 执行无硬件 smoke test。

## 不在本阶段实现

- 自动下载并运行未签名更新（受签名保护的应用内更新由规格 16 定义）；
- 包管理器商店发布；
- 在 CI 中连接真实 NFC 硬件；
- 用 UPX 压缩二进制。

## 平台要求

### Windows

- 主程序能在全新用户环境启动；
- `libnfc.dll` 和依赖 DLL 可从应用私有目录加载；
- 外部 `.exe` 不依赖开发机 PATH；
- 路径含空格和非 ASCII 用户名时可运行；
- 安装/卸载不修改系统级 libnfc 配置。

### macOS

- dylib 使用正确的 `@rpath`/`@loader_path`；
- 先签名嵌套 dylib 和外部工具，再签名主应用；
- 完成 notarization 后用户无需关闭 Gatekeeper；
- arm64 产物不能混入 x86_64-only 依赖。

### Linux

- 在兼容性较好的基础环境构建；
- AppImage 或私有运行时避免依赖目标机器恰好安装相同 libnfc；
- 串口/USB 权限问题显示可操作错误，不尝试自动 sudo；
- tar.gz 版本保留可执行权限。

## Runtime 自检

应用“关于/诊断”页面至少显示：

- NFCX 版本和 commit；
- Go/Wails 构建信息；
- OS/architecture；
- libnfc 版本；
- 启用或检测到的驱动；
- mfoc/mfcuk 版本和可用性；
- 动态库加载错误；
- 当前设备 connstring，但隐藏不必要的用户路径信息。

## 自动化测试

- 三个平台运行 `go test ./...`；
- Wails production build；
- 启动应用或 headless self-check；
- 加载私有 libnfc；
- 枚举外部工具并执行 `--help`/版本检查；
- 校验 runtime manifest 和 SHA-256；
- 检查许可证文件齐全；
- macOS 使用 `codesign` 验证；
- 安装包解压后不存在开发机绝对路径。

## 手工验收

每个平台至少在一台没有开发环境的机器或干净虚拟机中：

1. 安装/解压并启动；
2. 打开诊断页面；
3. 连接支持的读卡器；
4. 寻卡并读取测试卡；
5. 保存 dump；
6. 在授权测试卡上完成一次非 block 0 写入和验证；
7. 至少在一个适用平台验证 mfoc 启动、取消和设备恢复。

## 验收标准

- [ ] 三个平台都有可下载产物和 SHA-256。
- [ ] 应用不依赖用户预装 libnfc 或 mfoc/mfcuk。
- [ ] 动态库和外部工具从应用私有目录加载。
- [ ] macOS 产物通过签名/公证验证。
- [ ] Windows/Linux 在干净环境能够启动。
- [ ] 第三方许可证和源码来源完整。
- [ ] 诊断页面足以定位驱动、动态库和引擎缺失问题。
- [ ] 发布说明准确列出已验证的读卡器、卡型和限制。
