# 规格 10A 实施记录：统一密钥恢复入口

日期：2026-09-13

## 实际完成内容

- 新增后端 `KeyRecoveryService`，固定编排五步：常见密钥、按需 Darkside、按需 Nested、Hardnested 能力判断、实际读取验证。
- Darkside 单阶段超时设置为 1 小时；Nested 为 30 分钟。两种外部任务继续复用既有设备交接、取消、进程树终止、原始日志和 libnfc 复验机制。
- 以“至少一个扇区存在已验证 Key A 或 Key B”作为 Nested 的种子条件；只有所有扇区都具备可验证密钥时才跳过 Nested。
- 当前没有 Hardnested 引擎、runtime 或 capability，流程第 4 步会明确显示“不可用”，并把已有密钥和 partial dump 保留下来。
- 最终结论不只看密钥表或外部工具输出。第 5 步重新通过 libnfc 逐扇区读取，以实际可读扇区数区分 complete、partial 和 failed。
- 主界面把三个面向用户的独立按钮合并为 `自动恢复密钥`，增加固定五步状态条、不可计算阶段的动态进度、授权确认、超时提示、结果弹窗与逐步失败原因。原来的单项后端 binding 暂时保留作兼容和诊断用途。
- 用户取消不显示失败弹窗；已经通过 libnfc 验证的密钥仍按原有策略保留。
- 增加完整覆盖、部分种子、无种子、Darkside 失败、阶段切换、取消传播和 DTO 隐私测试。

## 实现过程中遇到的问题

### 外部 pipeline 的阶段边界不是 UI 步骤边界

MFCUK pipeline 内部会在验证种子后直接启动 MFOC。统一编排层现在在收到 MFOC 阶段事件时，先明确结束第 2 步，再开始第 3 步，避免 UI 同时把 Darkside 和 Nested 显示为运行中。

### 取消可能发生在已经取得种子之后

如果 MFCUK 已恢复并验证种子、随后 Nested 被取消，密钥覆盖数已经大于零。不能因为存在部分结果就把取消降级为普通 partial；实现会继续向上传播取消，同时保留已验证结果。

### Wails 绑定需要在前端编译前重新生成

新增 `StartKeyRecovery` 后，旧的生成文件没有该导出。执行 Wails 绑定生成后，TypeScript 才能校验统一请求 DTO。该问题已解决。

## 阻塞或未完成事项

- 本记录初次完成时 Hardnested 尚未实现；规格 10B 已在同日补齐固定 adapter、runtime、capability 和输出验证，并接入第 4 步。
- 真实 PN532 UART + 授权 Classic 1K 卡的完整手工验收仍需用户执行。
- 除上述两项外无阻塞。

## 验收证据

自动化命令和手工记录位置见 `docs/evidence/10a-unified-key-recovery/README.md`。
