# 规格 02 实施记录：固定 libnfc 工具链

日期：2026-09-12

## 实际完成内容

- 固定官方 libnfc 1.8.0 release、完整 commit SHA 和源码包 SHA-256。
- 将首阶段驱动集合收窄为 `pn532_uart`，不引入当前硬件不需要的 libusb 或 PC/SC 依赖。
- 使用发布包自带的 Autotools `configure` 建立 macOS arm64 可复现构建脚本。
- 构建脚本在独立临时目录解压和编译，只把开发 SDK 与应用动态库复制到各自输出目录。
- 禁用系统配置文件读取，保留 `LIBNFC_DEVICE` 等环境变量入口，不修改 `/etc/nfc` 或其他系统级配置。
- 建立可追踪补丁目录和顺序文件，并加入 `LIBNFC_DEVICE` 在关闭 conffiles 时仍可列出的兼容性补丁。
- 增加依赖检测、源码校验、驱动检查、Mach-O 架构/依赖/install-name 检查和最小 C smoke test。
- 增加显式 PN532 UART 硬件验收命令，串口 connstring 只从调用参数传入。
- 在生成的 runtime 中附带上游许可证正文和源码锁定信息。

## 实现过程中遇到的问题

### macOS 动态库安装名称

libnfc 的 libtool 构建默认把安装前缀写入 dylib install name。构建脚本在复制运行时前将版本化 dylib 的 ID 改为 `@rpath/libnfc.6.dylib`，并通过 `otool` 检查依赖，防止应用运行时依赖开发机器上的绝对路径。

### 构建环境网络受限

首次源码下载在受限网络环境中无法解析 GitHub。取得网络许可后已从官方 release 下载并核验 SHA-256。后续构建使用经过相同哈希检查的本地缓存，也可通过 `NFCX_LIBNFC_ARCHIVE` 使用预先下载的源码包。

### 受限环境中的 CPU 数量探测

macOS 通常可通过 `sysctl hw.ncpu` 获取并行构建任务数，但受限沙箱会拒绝该调用。脚本改为优先使用 POSIX `getconf`，再降级到 `sysctl` 和安全默认值，因此不会因非关键的性能探测中断构建。

### release archive 的 Git 版本探测

上游 `configure` 即使处理不含 `.git` 的 release archive 也会尝试运行 `git describe`。脚本只在 configure 阶段屏蔽 Git，避免误用外围 NFCX 仓库的 revision；libnfc 的来源身份统一由锁文件中的 tag、commit 和 archive SHA-256 确定。

### C 源文件路径进入动态库

初版使用 out-of-tree build，libnfc 的日志宏通过 `__FILE__` 将临时源码绝对路径写入 dylib。最终改为在一次性解压目录内原地构建，使这些字符串保持相对路径，并将 `/var/folders` 一并纳入私有路径拒绝规则。

### `LIBNFC_DEVICE` 被 conffiles 开关意外屏蔽

硬件首次验收时，调试日志显示环境变量已进入 `user_defined_devices`，但没有任何 UART 驱动日志。检查 libnfc 1.8.0 源码后确认：`nfc_context_new` 在 `ENVVARS` 下填充设备，而 `nfc_list_devices` 却只在 `CONFFILES` 下消费同一数组。关闭文件配置因此同时破坏了环境设备选择。NFCX 添加一份最小补丁，移除该错误条件，并用 synthetic connstring smoke test 固定此行为。

### macOS UART 异步发送导致 ACK 超时

环境设备补丁生效后，libnfc 能打开 FT232RL 并设置 115200 baud，但 PN532 对 SAMConfiguration 不返回 ACK。上游尚未合并的 PR #633 已记录相同的 macOS PN532 UART 问题。NFCX 仅提取其最终版的 macOS 发送策略：逐字节写入并按 115200 baud 间隔 9 微秒；其他 POSIX 平台仍保持单次 `write`。

应用补丁后的真实硬件复测成功：PN532 对初始化及轮询命令均返回 ACK，`nfc-list` 输出 `NFC device: user defined device opened` 并正常退出。测试时未放置卡片，因此各类 target 数量为零。

## 阻塞或未完成事项

- PN532 + FT232RL 的打开、初始化和无卡寻卡已通过真实硬件验收。放置测试卡后的 UID/ATQA/SAK 读取以及重复插拔验收尚未执行；这些测试不会进入普通自动化流程。
- Windows amd64 和 Linux amd64 的构建实现按规格留待后续原生平台工作，本阶段只定义运行时目录命名。
- USB 和 PC/SC 读卡器支持经用户确认暂不纳入本阶段。

## 验收命令

```sh
make toolchain-check
make libnfc-build
make libnfc-verify
make libnfc-smoke
make libnfc-repeatability-check
```

真实硬件验收：

```sh
make libnfc-hardware-smoke LIBNFC_DEVICE='pn532_uart:<serial-port>'
```

自动化验收结果见 [`docs/evidence/02-libnfc-toolchain/README.md`](../evidence/02-libnfc-toolchain/README.md)。
