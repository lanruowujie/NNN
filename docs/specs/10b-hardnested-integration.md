# 规格 10B：MFOC-Hardnested 集成

## 目标

在至少存在一把 NFCX 已验证密钥、但 Nested 无法恢复所有扇区时，通过外部 `mfoc-hardnested` 执行 Hardnested，并把复验结果交回统一密钥恢复流程。

## 工具与能力

- 固定 `nfc-tools/mfoc-hardnested` 0.10.9 的精确 commit、源码 SHA-256 与 GPL-2.0-or-later 许可证。
- 使用与 NFCX、MFOC、MFCUK 相同的固定 libnfc 1.8.0 SDK 构建。
- macOS arm64 使用系统 liblzma；后续平台必须在原生 runner 固定对应依赖。
- 第一阶段只对 PN532 UART 启用 Hardnested capability；未知设备默认关闭。

## 执行规则

- 仅在已有 NFCX 已验证 Key A 或 Key B 时启动。
- 使用参数数组传递 `-C -F`、去重后的 `-k` 种子和任务临时目录中的 `-O` 输出文件。
- `-C` 跳过工具自带常见密钥扫描，`-F` 强制 Hardnested；常见密钥和 Nested 已由前序步骤完成。
- 默认阶段超时为 6 小时，支持用户取消、进程树终止、5 秒心跳和原始 stdout/stderr。
- 退出码为零后仍须校验输出文件、dump 长度、UID/BCC、sector trailer 和访问控制位。
- 从通过结构校验的 dump 提取候选 Key A/Key B，再用 in-process libnfc 对每个候选重新认证；只有复验通过者进入密钥库。
- 无论成功、失败或取消，外部任务结束后都恢复 reader。最终仍由统一流程逐扇区实际读取。

## 自动化测试

- 版本与必需参数能力探测。
- `-C -F` 和去重种子的参数生成。
- 输出缺失、损坏、错误长度与非零退出。
- 1K dump 结构校验与逐密钥 libnfc 复验。
- capability、授权和种子前置条件。
- Nested 失败后进入 Hardnested，完整恢复后继续第 5 步。
- 超时和取消传播。

## 手工验收

- 使用已获授权、Nested 不适用但 Hardnested 可恢复的 Classic 1K 测试卡。
- 第 3 步失败或不完整后，第 4 步变为运行中，进度条保持动态并显示心跳。
- 原始日志能够单独查看，主任务日志不复制其中可能出现的完整密钥。
- 完成后第 4 步显示密钥覆盖数，第 5 步用实际读取结果给出最终结论。
- 取消后原生进程退出、reader 恢复且已验证结果保留。
