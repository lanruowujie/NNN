# 规格 05 实施记录：MIFARE Classic 基础读写

日期：2026-09-12

## 实际完成内容

- 在 `internal/mifare` 实现 Classic 1K/4K 几何模型，覆盖 sector 数、block 数、dump 大小、sector 首 block、trailer、每扇区 block 数以及 block 到 sector 的映射。
- 保留 `Key [6]byte` 的固定长度类型，并增加严格复制的 `KeyFromBytes`，拒绝所有非 6 字节输入。
- 在 libnfc 底层集中定义并实现 `0x60` Key A 认证、`0x61` Key B 认证、`0x30` 读取和 `0xA0` 写入；业务和 workflow 不包含这些命令字节。
- C shim 在 opaque handle 内保存当前选中的 `nfc_target`、认证状态和已认证 sector；CardInfo 重选、关闭、目标丢失和命令失败都会使认证失效。
- 认证使用固定 libnfc 的 easy framing 和当前 UID 最后 4 字节，Key A/Key B 在 Go libnfc backend 内映射为命名命令。
- 读取严格返回 16 字节；兼容 PC/SC 后端可能附加的两个状态字节，但只向 Go 复制前 16 字节数据。
- 普通 `Reader.WriteBlock` 和 `ClassicIO.WriteBlock` 均拒绝 block 0 与 sector trailer。底层 C 写原语仍可表达 trailer block，后续只能由显式受保护流程调用。
- 普通数据块写入与立即回读在同一个 Reader 操作锁内完成；内容不一致返回独立的 `ErrVerificationFailed`。
- 增加 `ErrCardChanged`、`ErrNotAuthenticated` 和 `ErrVerificationFailed`，并贯通 native、Go 与 GUI DTO 错误名映射。
- 在 `internal/workflow` 增加 `ClassicIO`：通过 `DeviceManager.WithReader` 暂停轮询并独占设备，重新选卡、核对 UID/ATQA/SAK、认证，再执行单块读写。
- 增加显式硬件测试：默认用 Key A 认证并读取 block 0 和普通数据 block 1，再用派生的错误 Key A 验证认证失败分类，并确认普通 API 拒绝 block 0；只有同时设置所有权确认、目标数据 block 和 16 字节测试数据时才会写卡，并在测试结束前恢复原内容。

## 实现过程中遇到的问题

### Classic 在场检测会清除认证

固定版 libnfc 1.8.0 的 PN53x `target_is_present` 对 MIFARE Classic 通过重新选择指定 UID 来确认在场，上游源码明确说明这会丢失已认证 sector 的 Crypto1 状态。因此实现只在命令失败后的错误分类中调用它，不在认证成功后的读写前调用。成功的认证后读写连续执行；若移卡或换卡，读写命令会失败，再通过重选结果区分无卡、同卡认证失效和卡片身份变化。

### PC/SC 读取长度

libnfc 的 MIFARE helper 允许 Classic read 返回 16 字节数据，或返回 16 字节数据加 2 字节 PC/SC 状态字。C shim 因此使用足够大的临时响应缓冲区，验证长度为 16 或 18 后只复制数据部分，避免 PN532 路径正常但后续 PC/SC 接入失败。

### sector trailer 无法使用普通全块回读语义

Classic trailer 的 Key A 通常不可直接读回，所以不能把普通数据块的 16 字节全等校验原样用于 trailer。规格 05 的普通写 API 因而拒绝 trailer；底层能力保留，待规格 06 定义 access bits 校验、写入顺序和新密钥重认证后再通过专门流程开放。

### PN532 重选前不能主动 deselect

首次真实只读验收通过后，启用写入保护开关的测试在写前第二次 `CardInfo` 返回无卡。此时尚未调用写命令，卡片内容未改变。原因是重选前的 `nfc_initiator_deselect_target` 会通过 PN53x InRelease 将 Classic 卡置于 HALT，紧接着的普通寻卡不能立即重新发现它。修复后不再先发送 deselect；首次发现执行普通 passive selection，已有目标则通过 libnfc 的指定 UID 在场检查完成重选并清除 NFCX 保存的认证状态。

## 阻塞或未完成事项

- 自动化、竞态、CGO 链接和显式硬件测试编译均已通过。
- 用户已完成真实 PN532 + Classic 卡的默认 Key A 认证以及 block 0、block 1 读取验收。
- 修复 PN532 deselect/HALT 问题后，用户已完成 block 1 的真实写入、立即回读验证和原内容恢复验收。
- 用户于 2026-09-12 11:10 再次运行快速硬件测试并通过，确认派生错误 Key A 返回认证失败，普通 API 拒绝 block 0。
- 移卡、换卡和设备断开的可判定错误由无硬件自动化测试覆盖。
- 整卡 dump/restore 属于规格 06；密钥扫描和扇区工作台 GUI 属于规格 07，本阶段未实现。

规格 05 范围内无阻塞或未完成事项。

## 验收命令

不需要硬件：

```sh
go test ./...
go test -race ./internal/mifare/... ./internal/nfc/... ./internal/device/... ./internal/workflow/... ./app/...
go vet ./...
make frontend-build
make libnfc-binding-test
```

显式只读硬件验收，默认 Key A 为 `FFFFFFFFFFFF`，可通过 `NFCX_TEST_KEY_A` 覆盖：

```sh
make libnfc-binding-hardware-smoke LIBNFC_DEVICE='pn532_uart:<serial-port>'
```

显式写入并恢复专用测试 block：

```sh
NFCX_HARDWARE_WRITE_CONFIRM=I_OWN_THIS_CARD \
NFCX_TEST_WRITE_BLOCK='<non-zero-data-block>' \
NFCX_TEST_WRITE_HEX='<32-hex-digits>' \
make libnfc-classic-hardware-test LIBNFC_DEVICE='pn532_uart:<serial-port>'
```

自动化验收结果与覆盖范围见 [`docs/evidence/05-mifare-classic-io/README.md`](../evidence/05-mifare-classic-io/README.md)。
