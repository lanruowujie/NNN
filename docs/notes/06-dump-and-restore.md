# 规格 06 实施记录：Dump、Restore 与安全校验

日期：2026-09-12

## 实际完成内容

- 在 `internal/mifare` 增加 byte-level certainty dump 模型。每个 block 都保留 `unread`、`read`、`auth_failed`、`read_failed`、`synthetic` 或 `verified` 状态，并使用 16-bit known mask 区分真实零值和未知字节。
- raw dump 只接受 Classic 1K 的 1024 字节或 Classic 4K 的 4096 字节；任何未知字节都会使 `Raw()` 返回 `ErrIncompleteDump`，不会产生零填充的伪完整文件。
- 实现 4-byte UID BCC 计算和验证。7/10-byte UID 或没有 NFCX 元数据的独立 raw 文件不套用 4-byte 布局规则，而是明确返回 `not_applicable`。
- 实现 sector trailer access bits 的完整编解码、反码关系校验、1K/4K block group 映射，以及数据块和完整 trailer 写入所需的 Key A/Key B 权限判定。
- `DumpService` 在一次 `DeviceManager.WithReader` 独占操作中逐扇区验证调用者提供的 Key A/Key B，并在每次 block 操作前重新确认 UID、ATQA 和 SAK。
- trailer 的 Key A 永远视为卡片读取时被遮蔽，只有该 sector 的 Key A 实际认证成功后才补入，并记录 `authenticated_key_a` 来源。
- 根据 trailer access condition 判断 Key B 是可读数据还是被遮蔽的密钥。被遮蔽时只有实际认证成功的 Key B 才可补入；否则该 trailer 和整卡保持 partial。
- 完整 dump 可保存为不带私有字节的 `.bin`/`.mfd` raw 文件，并写入权限为 `0600` 的并列 `<name>.nfcx.json` 元数据。partial dump 只能保存 JSON 工程。
- 元数据包含 schema version、完整性、卡片/设备信息、时间、逐 block 状态/known mask/错误/密钥来源和逐 sector 已验证密钥，可无损 round trip。
- raw 文件加载时校验长度、sidecar 一致性、所有 trailer access bits，并在 sidecar 明确源 UID 为 4-byte 时校验 BCC。
- Restore 预检在任何写操作前完成源 dump 完整性、容量、BCC、源和目标 access bits、目标卡身份、全部目标 sector 密钥及实际写权限检查。
- 写入计划固定为“全卡普通数据块在先、所有 sector trailer 在后”，block 0 固定为 `skipped_protected`，没有任何启用它的参数。
- 普通数据块继续调用原有 `Reader.WriteBlock`，保持 native 写入与立即全块回读验证。trailer 使用独立的 `ClassicTrailerWriter` 能力，普通写 API 仍然拒绝 trailer。
- trailer 写入后使用新 Key A 重新认证并比较 bytes 6..9。Key B 可读时比较 bytes 10..15；Key B 被遮蔽时使用新 Key B 重新认证证明写入结果。
- restore 遇到第一个错误立即停止。逐块报告区分未尝试、写前条件失败、写命令结果未知、已写但验证失败和已验证，并提供已写、已验证、未尝试及失败 block 清单。
- 增加不暴露完整密钥的 dump/restore GUI DTO。实际工作台交互仍按约定留给规格 07。
- 增加显式硬件 dump/restore 测试及 Makefile 入口。测试只有在专用 build tag、硬件开关和 `I_OWN_THIS_CARD` 写入确认同时存在时才执行整卡写入。

## 实现过程中遇到的问题

### trailer 读取结果不能直接视为完整内容

MIFARE Classic 正常读取 sector trailer 时 Key A 始终返回零；Key B 也会根据 access condition 返回零或作为数据可读。因此仅保存 16-byte 读取响应会把“被遮蔽”错误地变成“真实零密钥”。实现使用 byte-level known mask，并只从同一 sector 的实际成功认证结果补充密钥。

### trailer 不能沿用普通块的全 16 字节回读比较

Key A 无法通过普通 READ 返回，因此 trailer 验证分为可读 access bytes 比较和新密钥认证。Key B 是否直接比较由新 trailer 自己的 access condition 决定。

### 只验证源 access bits 不足以保证目标可写

目标卡当前 access condition 决定哪个密钥能够写普通块或修改完整 trailer。预检因此也读取并解码目标 trailer，按 block group 选择已验证 Key A/Key B。若完整 16-byte trailer 写入只允许修改部分字段，则在零写入状态下拒绝任务。

access condition 语义依据 NXP 的 MIFARE Classic EV1 1K/4K 数据手册，没有从 Python 原型复制不完整的权限表。

### 写命令失败不一定能证明卡片未改变

读卡器可能在卡片已接受写入后丢失响应。报告不会把这类错误算作“确定未写”，而是标为 `write_outcome_unknown`；只有 native 写成功后发生回读/重新认证失败，才标记为“已写但验证失败”。

## 阻塞或未完成事项

- 自动化、竞态、静态检查、默认构建、固定 libnfc CGO 构建和显式硬件测试编译均已通过。
- 用户已在真实 PN532 UART + 授权测试卡上完成只读 dump 验收，确认整卡读取、持久 raw/sidecar 保存及重新加载校验成功。
- 用户随后以 `I_OWN_THIS_CARD` 显式确认完成真实整卡 restore，命令返回 `OK`；该流程完成了预检、跳过 block 0、普通数据块优先、trailer 最后写入及逐块验证。
- 完整工作台 GUI、任务窗口和文件选择交互属于规格 07，本阶段只提供可序列化 DTO。
- 按用户确认，block 0 本阶段始终跳过；特殊 UID 卡写入留给规格 11。

规格 06 范围内无阻塞或未完成事项。

## 验收命令

不需要硬件：

```sh
go test ./...
go test -race ./internal/mifare/... ./internal/nfc/... ./internal/device/... ./internal/workflow/... ./app/...
go vet ./...
make frontend-build
make libnfc-binding-test
```

只编译显式硬件测试，不访问卡片：

```sh
CGO_ENABLED=1 go test -run '^$' -tags 'libnfc libnfc_hardware' ./internal/workflow
```

真实整卡读取并把同一内容安全写回授权测试卡：

```sh
NFCX_TEST_DUMP_PATH='/absolute/path/to/test-card-backup.bin' \
make libnfc-dump-hardware-test \
  LIBNFC_DEVICE='pn532_uart:<serial-port>'
```

以上命令只读卡、保存并重新加载文件；测试末尾显示 `SKIP` 表示按设计没有进入写卡阶段。确认 raw 与 sidecar 已保留后，再执行整卡写回：

```sh
NFCX_HARDWARE_RESTORE_CONFIRM=I_OWN_THIS_CARD \
NFCX_TEST_DUMP_PATH='/absolute/path/to/test-card-backup.bin' \
make libnfc-dump-restore-hardware-test \
  LIBNFC_DEVICE='pn532_uart:<serial-port>'
```

`NFCX_TEST_DUMP_PATH` 必须是希望长期保留的绝对备份路径；对应 `.nfcx.json` 会写在旁边。非默认 Key A/Key B 可分别通过 `NFCX_TEST_KEY_A` 和 `NFCX_TEST_KEY_B` 传入 12 个十六进制字符。

自动化验收结果与覆盖范围见 [`docs/evidence/06-dump-and-restore/README.md`](../evidence/06-dump-and-restore/README.md)。
