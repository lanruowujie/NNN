# 规格 06 自动化验收证据：Dump、Restore 与安全校验

日期：2026-09-12  
平台：macOS arm64  
Go：1.25.1  
libnfc：固定版本 1.8.0

## 已通过检查

```sh
GOCACHE=/tmp/nfcx-go-cache go test ./...
GOCACHE=/tmp/nfcx-go-cache go vet ./...
GOCACHE=/tmp/nfcx-go-cache go test -race \
  ./internal/mifare/... ./internal/nfc/... ./internal/device/... \
  ./internal/workflow/... ./app/...

GOCACHE=/tmp/nfcx-go-cache \
PKG_CONFIG_PATH="$PWD/build/toolchain/darwin-arm64/sdk/lib/pkgconfig" \
DYLD_LIBRARY_PATH="$PWD/runtime/darwin-arm64" \
LD_LIBRARY_PATH="$PWD/runtime/darwin-arm64" \
CGO_ENABLED=1 go test -tags libnfc ./...

GOCACHE=/tmp/nfcx-go-cache \
PKG_CONFIG_PATH="$PWD/build/toolchain/darwin-arm64/sdk/lib/pkgconfig" \
DYLD_LIBRARY_PATH="$PWD/runtime/darwin-arm64" \
LD_LIBRARY_PATH="$PWD/runtime/darwin-arm64" \
CGO_ENABLED=1 go test -run '^$' -tags 'libnfc libnfc_hardware' \
  ./internal/workflow
```

最后一条命令只编译硬件测试，输出为 `ok ... [no tests to run]`，没有访问或写入卡片。

## 自动化覆盖

- Classic 1K/4K raw dump 的合法和非法长度；
- 4-byte UID BCC 正确、错误及 7/10-byte/未知来源不适用场景；
- 所有 4096 种 access-bit group 组合的编解码 round trip；
- access bits 三组反码关系的独立损坏；
- Classic 4K 大扇区 block-to-access-group 映射；
- sector 0、普通 sector 和 sector 39 trailer 结构验证；
- Key A 被遮蔽后的成功认证补全和来源记录；
- Key B 可读与被遮蔽两种语义；
- 缺少真实密钥时保持 partial，且 raw 导出被拒绝；
- partial JSON project 和完整 metadata round trip；
- `.bin`/`.mfd` raw 与 sidecar 保存、加载及一致性检查；
- GUI DTO 不暴露完整密钥；
- restore 在源 access bits、BCC、完整性、容量或目标认证失败时零写入；
- block 0 固定跳过；
- 全部普通数据块先写、全部 trailer 后写；
- 每个成功写入 block 都标记回读/重新认证验证结果；
- 中途失败时准确区分已写、已验证、未尝试和结果未知 block；
- 普通 Reader 写入仍拒绝 trailer，受保护 trailer 能力不会执行普通全块回读语义；
- 卡片中途更换立即返回 `ErrCardChanged`，后续 block 保持未访问。

测试数据由测试代码确定性生成，只使用递增 block 编号、transport access bits 和默认测试密钥，不包含真实卡片数据。

## 真实硬件状态

用户已在真实 PN532 UART + 授权测试卡上依次执行并确认以下两个 Makefile 目标返回 `OK`：

```sh
make libnfc-dump-hardware-test
make libnfc-dump-restore-hardware-test
```

第一步验证了整卡读取、持久 raw/sidecar 保存和重新加载；第二步验证了 restore 预检、block 0 保护、普通数据块优先、trailer 最后写入，以及每个写入 block 的回读或新密钥验证。

全卡 restore 会写入除 block 0 外的所有 block，测试入口要求用户在自有测试卡上通过以下三重条件显式启用：

- `libnfc_hardware` build tag；
- `NFCX_WORKFLOW_HARDWARE=1`；
- `NFCX_HARDWARE_RESTORE_CONFIRM=I_OWN_THIS_CARD`。

执行 restore 时还必须通过 `NFCX_TEST_DUMP_PATH` 指定持久备份路径，确保测试失败后 raw dump 和 sidecar 不会随测试临时目录删除。

Makefile 的 `libnfc-dump-restore-hardware-test` 目标负责设置前两个条件，并强制检查第三个条件。本次真实硬件验收已通过，规格 06 无剩余硬件阻塞。
