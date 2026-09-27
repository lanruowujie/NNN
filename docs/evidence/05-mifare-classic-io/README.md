# 规格 05 自动化验收证据：MIFARE Classic 基础读写

日期：2026-09-12  
平台：macOS arm64  
libnfc：固定版本 1.8.0，`pn532_uart` 驱动

## 已通过检查

```sh
go test ./...
go vet ./...
go test -race ./internal/mifare/... ./internal/nfc/... ./internal/device/... ./internal/workflow/... ./app/...
CGO_ENABLED=1 go test -tags libnfc ./...
CGO_ENABLED=1 go test -run '^$' -tags 'libnfc libnfc_hardware' ./internal/nfc/libnfc
```

最后一条命令只编译显式硬件验收测试，不访问或写入卡片。

自动化覆盖：

- Classic 1K 全部 16 sectors/64 blocks 的双向几何映射；
- Classic 4K 全部 40 sectors/256 blocks 的双向几何映射和大扇区边界；
- 1024/4096 字节 dump 大小；
- 非法 layout、sector、block 和 key 长度；
- Key A `0x60` 与 Key B `0x61` 的底层映射；
- 卡片重选后的认证失效；
- workflow 开始时 UID/ATQA/SAK 变化立即返回 `ErrCardChanged`；
- 认证失败、无卡和设备断开错误透传；
- block 0 和 trailer 普通写保护；
- 写入后立即读取同一 block；
- 回读不一致返回独立的 `ErrVerificationFailed`；
- CGO shim 使用固定 SDK 编译并通过已有资源生命周期测试。

## 真实硬件状态

用户随后使用准确的 PN532 UART connstring 完成了只读验收：默认 Key A 认证以及 block 0、block 1 读取通过。

首次启用写入保护开关时，测试在写前第二次 `CardInfo` 返回无卡，尚未调用写命令。该结果定位到重选前主动 deselect 会让 PN532 把 Classic 卡置于 HALT；实现移除这一步后，用户重跑快速硬件测试并通过，确认 block 1 写入、立即回读验证和原内容恢复成功。

2026-09-12 11:10，用户在加入错误密钥和 block 0 保护检查后再次执行：

```sh
NFCX_HARDWARE_WRITE_CONFIRM=I_OWN_THIS_CARD \
NFCX_TEST_WRITE_BLOCK=1 \
NFCX_TEST_WRITE_HEX='00112233445566778899AABBCCDDEEFF' \
make libnfc-classic-hardware-test \
  LIBNFC_DEVICE='pn532_uart:/dev/cu.usbserial-A50285BI'
```

结果：

```text
ok github.com/BennyThink/NFCX/internal/nfc/libnfc 0.859s
```

该次测试同时通过：

- 正确 Key A 认证；
- block 0 和 block 1 读取；
- 派生错误 Key A 返回 `ErrAuthenticationFailed`；
- 普通 API 拒绝 block 0；
- block 1 写入及立即回读一致；
- 恢复 block 1 原内容并再次回读一致。

规格 05 的真实硬件手工验收通过。移卡、换卡和拔出设备的可判定错误由自动化测试覆盖。
