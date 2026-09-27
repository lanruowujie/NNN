# 规格 10 实施记录：MFCUK 到 MFOC 自动流程

日期：2026-09-13

## 实际完成内容

- 固定上游 MFCUK 0.3.8 tag、commit、源码 SHA-256 和 GPL-2.0-or-later 许可证，增加 macOS arm64 构建、校验、runtime 打包和签名流程。MFCUK 与 NFCX、MFOC 使用同一份固定的 patched libnfc 1.8.0。
- 维护一个不改变 Darkside 算法的最小补丁。上游完成密钥恢复并成功写入结果结构后，额外输出 `NFCX_RESULT key=<A|B> sector=<n> value=<12 hex>`；补丁随 runtime 和对应源码信息一起分发。
- 设备 capability 增加独立 `Darkside` 字段。经用户确认，`pn532_uart` profile 同时允许尝试 Darkside 和 Nested；未知设备仍默认全部关闭。
- 实现 `MFCUKEngine`：仅从受控 runtime 解析固定二进制，检查精确版本、恢复选项和 NFCX 机器结果能力，通过参数数组运行 `-C -R 0:A -s 250 -S 250 -v 2`，并传递精确 `LIBNFC_DEVICE`。
- 机器结果严格解析并按 sector、key type、value 去重。自然语言中的 `recovered KEY` 不会被当作成功结果；候选 artifact 的结构校验也不会被表达成密钥已验证。
- 实现两阶段 pipeline service。第一阶段释放 reader 运行 MFCUK，恢复后用 in-process libnfc 逐个验证所有候选；至少一个候选通过后才调用现有 MFOC workflow。MFOC 会再次释放和恢复 reader，并继续使用既有的 dump 结构校验、逐 sector 密钥复验和无覆盖合并。
- pipeline 使用统一的 60 分钟总超时，MFCUK 与 MFOC 各有 30 分钟阶段上限。用户取消、阶段超时和进程失败都沿用现有进程树终止与设备恢复机制。
- MFCUK 已验证的种子会立即安全并入密钥库。因此第二阶段失败时，第一阶段已经由 libnfc 证明有效的密钥仍会保留，而 pipeline 整体仍报告失败。
- 最终密钥复验完成后，NFCX 不信任外部 dump 作为工作副本，而是重新通过 libnfc 读取卡片并沿用工作台的冲突安全合并逻辑。
- GUI 增加始终可见的 `MFCUK → MFOC` 入口、授权与不保证成功提示、两阶段状态、统一取消以及按引擎标记的原始日志。运行时或前置条件不满足时入口保持禁用并提供状态提示；只有两个固定引擎可用，且设备为 PN532 UART、卡片为 Classic 1K、设备空闲、没有已验证密钥时才启用。

## 实现过程中遇到的问题

### 上游没有稳定的结构化结果接口

MFCUK 0.3.8 的自然语言输出包含进度中的零值和最终 `recovered KEY` 文本，直接正则匹配容易把非最终内容误判为成功。实现采用随源码记录的最小机器输出补丁，并要求运行时可用性检查确认该能力存在。

### MFCUK 帮助命令以非零状态退出

固定版本的 `mfcuk -h` 打印正确版本和帮助后返回失败状态。适配器只在该精确探测场景容许非零退出，并仍严格验证版本、恢复参数和机器结果标记；实际恢复任务继续要求零退出码。

### 两阶段需要两次独立设备交接

没有把两个外部进程包在一次长时间释放中。MFCUK 结束后必须先恢复 reader 验证候选，随后 MFOC 再执行第二次释放；自动化测试固定了 `release → reopen → verify → release → reopen → verify` 顺序。

### Wails 构建在受限沙箱中隐藏原生编译错误

直接执行等价 Go/CGO 编译成功，允许 macOS 原生打包工具在沙箱外运行后 Wails 构建和应用签名完成。该问题属于开发环境权限，不是 NFCX 源码或工具链缺陷。

### GUI 运行时版本识别过于严格

首次硬件验收时，MFCUK 可执行文件存在，但 GUI 自检只接受单一的人类可读标题格式，导致入口提示 `version output is not recognized` 并保持禁用。补丁现额外输出 `NFCX_VERSION 0.3.8` 机器版本行，适配器优先解析该行，同时兼容绝对路径、Windows `.exe`、`version` 和 `v` 前缀。打包后已从 GUI 确认状态为 `MFCUK 0.3.8 → MFOC 0.10.7`。

### Darkside 长时间静默导致界面看似卡死

MFCUK 在 `RECOVER: 0` 后可能长时间停留在恢复循环，原始 `-v 2` 输出没有尝试次数，而攻击事件的详细消息此前又被通用“运行中”文案覆盖。现在外部进程每 5 秒发送一次包含已用时和阶段剩余超时的心跳；MFCUK 补丁每 25 次认证输出一条不含密钥的 `NFCX_PROGRESS` 记录，GUI 将其显示为实际尝试次数，并对无法计算百分比的任务使用往返动画而非静止的 2% 进度条。

## 阻塞或未完成事项

- 自动化测试、macOS arm64 MFCUK 构建和应用打包无阻塞。
- 真实 PN532 + FT232RL、无已知密钥授权 Classic 1K 卡片的完整 Darkside → Nested 手工验收仍待用户执行。
- Windows amd64 与 Linux amd64 的实际 MFCUK 构建和签名按规格 12 在原生 runner 完成；当前脚本会明确拒绝未实现平台。

## 验收命令

```sh
GOCACHE=/tmp/nfcx-go-cache go test ./...
GOCACHE=/tmp/nfcx-go-cache go vet ./...
GOCACHE=/tmp/nfcx-go-cache go test -race ./app ./internal/attack ./internal/device ./internal/keys ./internal/workbench ./internal/workflow
NFCX_MFCUK_ARCHIVE=/tmp/nfcx-mfcuk-0.3.8.tar.gz make mfcuk-build
make mfcuk-verify
make mfoc-verify
GOCACHE=/tmp/nfcx-go-cache make frontend-build
GOCACHE=/tmp/nfcx-go-cache make libnfc-binding-test
```

手工验收步骤和记录位置见 `docs/evidence/10-mfcuk-pipeline/README.md`。
