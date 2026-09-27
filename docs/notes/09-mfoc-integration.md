# 规格 09 实施记录：MFOC Nested 集成

日期：2026-09-13

## 实际完成内容

- 固定上游 MFOC 0.10.7 tag、完整 commit 和源码 SHA-256，记录 GPL-2.0-or-later 许可证与对应源码地址；增加 macOS arm64 可复现构建、校验和应用 runtime 打包脚本。MFOC 与主进程使用同一份 patched libnfc 1.8.0，应用包内通过 `@executable_path` rpath 加载。
- 新增保守的设备 capability 模型。未知设备默认不允许 Nested；第一阶段仅 `pn532_uart` profile 标为可尝试，且适配器在设备真正释放后再次核对实际 connstring 的 capability。
- 外部任务请求增加期望卡片。在停止轮询且仍持有设备锁时，关闭 reader 前主动重新寻卡并比较完整 UID、ATQA、SAK；不一致时不启动 MFOC，但仍重置并恢复 reader。
- 实现 `MFoCEngine`。它只从受控 runtime 解析固定程序，实际检查版本和所需 CLI 选项，使用参数数组启动，将卡片作用域内已经验证的 Key A/Key B 按值去重后分别通过重复的 `-k <key>` 参数传入，并使用任务专用 `-O` 输出路径。精确 connstring 通过子进程环境传递。
- stdout/stderr 原始字节保留在内部诊断结果，GUI 实时日志在流式分片重组后隐藏所有连续 12 位十六进制密钥。有限进度解析支持任意 chunk 边界，只报告正在处理的 sector，不根据自然语言判定成功。
- MFOC 正常退出后严格检查输出存在、与目标卡容量相符、raw dump 可解析、layout 匹配、所有访问位互补关系有效；4-byte UID 卡额外核对 block 0 UID 和 BCC。MFOC 0.10.7 对 1K 卡也固定写出 4096 字节结构，因此只在后 3072 字节全零时接受并截取前 1024 字节；非零尾部拒绝。校验后的最大 4 KiB artifact 在临时目录清理前复制到内存。
- 从每个 sector trailer 提取 Key A/Key B 候选。设备恢复后通过 in-process libnfc 重新选择目标卡并逐项认证；认证失败只进入诊断结果。恢复失败时不会导入任何候选。
- 密钥库增加原子无覆盖合并：槽位为空时导入、相同值时刷新来源和验证时间、不同值时报告冲突且既不覆盖旧验证也不把冲突候选加入全局字典。
- 复验结束后使用现有 `DumpService` 通过 in-process libnfc 重新读取卡片，不直接把 MFOC dump 当成可信工作副本。工作台合并只填补未知字节/密钥，保留已有数据和用户编辑并报告冲突；撤销历史同时吸收后台读入数据。
- Wails 增加 MFOC 状态和启动 binding。GUI 只有在固定版本可用时显示入口，并同时要求 Classic 1K/4K、`pn532_uart` Nested capability、至少一把已验证密钥和空闲设备；启动前明确要求用户确认卡片已获授权。任务界面区分外部输出校验、libnfc 密钥复验、冲突和工作台合并。

## 实现过程中遇到的问题

### GUI 预检与进程启动之间存在换卡窗口

仅依赖轮询 Snapshot 不能证明关闭 reader 前仍是同一张卡。设备释放事务因此增加了锁内 `Prepare` 阶段：先停止轮询、主动选择并比较卡片身份，再关闭句柄。即使 Prepare 拒绝任务，也会重开 reader，避免停留在半释放状态。

### MFOC 的 CLI 不区分种子属于 Key A 还是 Key B

NFCX 仍从带 sector 和 Key 类型的验证记录选择种子，但 MFOC 接收的是候选值集合。固定的 0.10.7 CLI 不支持字典文件参数，因此实现按值去重后生成重复的 `-k <key>` 参数；自动化测试确认只有 Key B 已验证时同样可以满足前置条件，类型信息不会被误认为外部结果验证。

### MFOC 日志包含完整密钥

上游会打印输入和发现的密钥，不能原样送入默认 GUI 日志。后端保留原始 stdout/stderr 以诊断进程问题，面向 GUI 的流使用带跨 chunk 缓冲的脱敏器，在任何 12 位十六进制序列可见前完成替换。密钥本身只通过专用扇区状态 UI 展示。

### 外部进程结束不等于密钥可信

规格 08 的 `succeeded` 只表示进程和结构化 artifact 成功。MFOC 工作流在 reader 重开后增加独立 `verifying_keys` 和 `merging_result` 阶段；Wails 顶层任务只会在复验和安全合并结束后发布 completed。

### 工作台可能包含用户修改

直接加载 MFOC dump 会覆盖当前修改。实现改为通过 libnfc 重新读取，并把新数据同时合并到 baseline、working copy 和撤销历史：未知内容可补齐，已知冲突只报告，不改变用户值。

## 真实硬件验收

2026-09-13，用户使用 PN532 + FT232RL 和自有 MIFARE Classic 1K 测试卡完成验收。默认全 F 卡首先验证了 MFOC 启动、设备交接和完整读取链路；随后使用随机密钥测试卡，只向 NFCX 提供一个已知 Key A 值。MFOC 选择 sector 0 作为 exploit sector，实际进入 Nested 流程并恢复此前未知的 Key B，所有扇区认证成功。修正上游 CLI 参数及 1K-in-4K 输出格式兼容后，恢复密钥通过 NFCX 的 in-process libnfc 复验，卡片可正常读取，用户确认规格 09 验收通过。

## 阻塞或未完成事项

- 规格 09 无代码、自动化测试或真实硬件验收阻塞。
- macOS arm64 的固定 MFOC 已完成源码校验、构建、动态依赖检查、rpath 检查和本地应用嵌套签名。
- Windows amd64 和 Linux amd64 的实际 MFOC 构建、依赖打包及签名按规格 12 在各自原生 runner 完成；当前脚本会明确拒绝未实现平台，不会生成未经验证的产物。

## 验收命令

```sh
GOCACHE=/tmp/nfcx-go-cache go test ./...
GOCACHE=/tmp/nfcx-go-cache go vet ./...
GOCACHE=/tmp/nfcx-go-cache go test -race \
  ./app ./internal/attack ./internal/device ./internal/keys \
  ./internal/workbench ./internal/workflow
NFCX_MFOC_ARCHIVE=/tmp/mfoc-0.10.7.tar.gz make mfoc-build
make mfoc-verify
GOCACHE=/tmp/nfcx-go-cache make libnfc-binding-test
GOCACHE=/tmp/nfcx-go-cache make frontend-build
GOCACHE=/tmp/nfcx-go-cache make build
```

详细自动化覆盖和手工验收边界见 `docs/evidence/09-mfoc-integration/README.md`。
