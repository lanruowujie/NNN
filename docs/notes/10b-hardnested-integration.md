# 规格 10B 实施记录：MFOC-Hardnested 集成

日期：2026-09-13

## 实际完成内容

- 选择并固定官方 `nfc-tools/mfoc-hardnested` 0.10.9、commit `a6007437405a0f18642a4bbca2eeba67c623d736`，记录精确源码 SHA-256 和 GPL-2.0-or-later 许可证。
- 新增 macOS arm64 构建、校验、runtime 打包和签名流程。原生工具链接 NFCX 固定的 libnfc 1.8.0 与系统 liblzma。
- 新增 `HardnestedEngine`。它只解析固定 runtime，严格检查版本及 `-C/-F/-k/-O` 能力，通过参数数组和精确 `LIBNFC_DEVICE` 启动。
- 新增 `HardnestedService`，复用 MFOC 已验证的 dump 结构校验、候选提取、逐密钥 libnfc 认证和冲突安全合并逻辑。
- PN532 UART capability 增加 Hardnested；未知 reader profile 仍关闭。
- 统一密钥恢复第 4 步现在会在 Nested 后覆盖不完整时探测并运行 Hardnested，默认超时 6 小时。成功、失败、取消和 runtime 缺失都有独立状态。
- GUI 更新授权提示、设备能力摘要和流程文案。原始外部输出只保留在用户显式打开的“原始日志”窗口，不再复制到主任务日志，避免默认暴露工具打印的完整密钥。
- 增加 engine、参数、workflow、capability 以及统一流程分支测试。

## 实现过程中遇到的问题

### 上游没有 release tag

仓库的包版本为 0.10.9，但没有可用于本次固定的 release tag。因此使用官方仓库 master 当前解析出的精确 commit，并记录该 commit 归档的 SHA-256，构建不会跟踪分支变化。

### macOS 可执行文件缺少运行时 rpath

上游构建能成功，但产物只记录 `@rpath/libnfc.6.dylib` 而没有 LC_RPATH。工具链脚本在安装前添加 `@executable_path`，随后校验链接关系，确保打包后能从同目录加载固定 libnfc。

### Hardnested 的输出包含完整密钥

上游 stdout 会打印认证密钥。NFCX 仍保留原始输出用于用户显式诊断，但主任务日志不再复制 raw chunk；结构化阶段事件和心跳不含密钥。最终信任依据是输出 dump 加 libnfc 复验。

### Hardnested 没有可靠百分比

工作量依赖采集到的 nonce 与候选状态数，不能稳定映射为 0–100%。GUI 使用第 4 步不确定进度动画，并沿用每 5 秒一次的已用时/剩余超时心跳。

### 无换行尾行的日志来源

复查时发现共享 MFOC 流解析器在 flush 无换行尾行时固定写入 `mfoc` 引擎名。现已让解析器保存实际 engine identity，并增加 Hardnested 尾行回归测试；该问题只影响日志来源标签，不影响结果文件或密钥验证。

## 阻塞或未完成事项

- 自动化实现、macOS arm64 原生构建与 runtime 校验无阻塞。
- 真实 PN532 UART + hardened nonce 授权卡的完整手工攻击仍待用户执行。
- Windows amd64 与 Linux amd64 构建继续按规格 12 在原生 runner 完成。

## 验收证据

命令和硬件检查表见 `docs/evidence/10b-hardnested-integration/README.md`。
