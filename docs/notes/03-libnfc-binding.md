# 规格 03 实施记录：libnfc C shim 与 Go Reader

日期：2026-09-12

## 实际完成内容

- 定义了与硬件实现无关的 `Reader`、`DeviceEnumerator`、`DeviceInfo`、`CardInfo`、严格 6 字节 `Key` 和 `KeyType`。
- 建立稳定的 NFCX 错误类别和 `OpError`，可通过 `errors.Is` 同时判断 NFC 错误类别与 context 取消原因。
- 在 `internal/nfc/libnfc` 中实现窄 C shim；`nfc_context`、`nfc_device` 和 `nfc_target` 均不离开该目录。
- C shim 实现了初始化、设备枚举、准确 connstring 打开、initiator 初始化、幂等关闭、abort 和 ISO14443A 寻卡。
- UID、ATQA、connstring 和预留块操作的所有输入输出均携带长度，并在复制进 Go 前验证边界。
- 认证、读块和写块只建立接口与 shim 签名，本阶段明确返回“不支持”，没有提前引入规格 05 的 MIFARE 命令实现。
- Go wrapper 串行化同一设备上的普通操作；关闭或 context 取消可以通过唯一允许并发执行的 abort 路径结束正在阻塞的设备命令。
- 默认构建使用无 CGO stub，普通 `go test ./...` 不依赖 libnfc；显式 `libnfc` build tag 才链接规格 02 生成的固定 SDK。
- 增加可配置 mock Reader 和 mock 设备枚举器，可模拟无卡、超时、设备断开、取消以及自定义块操作。
- 增加纯 Go、竞态、CGO 初始化、打开失败资源清理、UID 长度、关闭幂等和 100 次生命周期测试。
- 增加显式真实硬件验收命令；它会执行 100 次打开/关闭并读取自有测试卡的 UID、ATQA 和 SAK。

## 实现过程中遇到的问题

### 普通 CI 与 CGO 链接隔离

如果 CGO 文件默认参与构建，启用了 CGO 但没有本地 libnfc SDK 的 CI 会在链接前失败。binding 和 C shim 因此使用显式 `libnfc` build tag，并提供同接口的 unsupported stub。这样只有专门的 binding 命令需要原生工具链。

### 取消和设备句柄生命周期

`nfc_abort_command` 必须能与当前阻塞调用并发，但 device 又不能在 abort 或原命令仍使用它时释放。Go wrapper 使用操作锁、状态锁和关闭锁区分普通调用与 abort：关闭先拒绝新操作，仅在确有进行中操作时 abort，等操作退出后再释放 native handle。

### 打开阶段的 context 限制

libnfc 在 `nfc_open` 返回前不会暴露可用于 `nfc_abort_command` 的 device handle。因此 `Open` 会在原生调用前后检查 context，并在返回时 context 已取消的情况下立即释放新句柄；底层打开过程本身依赖固定驱动的有界超时，不使用不安全的线程强制终止。

### 设备友好名称

`nfc_list_devices` 可靠提供的是 connstring，友好名称通常只有打开设备后才能读取。枚举阶段不会为了名称逐个打开设备，`DeviceInfo.Name` 暂时回退为 connstring；规格 04 可以在选中设备真正打开后补充显示名称。

## 阻塞或未完成事项

- 真实 PN532 + FT232RL 的 Go binding 硬件验收尚未在本阶段自动执行，因为命令要求显式传入本机 connstring，并需要在读卡器上放置自有测试卡。
- GUI 设备列表、后台轮询、卡片出现/移除事件属于规格 04，未实现。
- MIFARE Classic 认证、读块和写块属于规格 05，未实现。

## 验收命令

不需要硬件：

```sh
go test ./...
go test -race ./internal/nfc/...
make libnfc-binding-test
```

显式真实硬件验收：

```sh
make libnfc-binding-hardware-smoke LIBNFC_DEVICE='pn532_uart:<serial-port>'
```

自动化验收结果见 [`docs/evidence/03-libnfc-binding/README.md`](../evidence/03-libnfc-binding/README.md)。
