# 规格 03 验收证据：libnfc C shim 与 Go Reader

日期：2026-09-12  
平台：macOS arm64  
libnfc：固定版本 1.8.0，`pn532_uart` 驱动

## 自动化验收

以下检查均通过：

```sh
go test ./...
go test -race ./internal/nfc/...
go vet ./...
make libnfc-binding-test
```

覆盖的关键行为包括：

- 默认构建不要求 CGO 或 libnfc SDK；
- C shim 可以初始化并退出固定版本 libnfc；
- 设备枚举、打开、关闭和寻卡只通过 Go 自有类型暴露；
- C 稳定错误码完整映射为可判定的 Go 错误；
- UID 4、7、10 字节均能复制，其他长度被拒绝；
- 打开失败后 context、device 和 opaque handle 资源计数恢复原值；
- 关闭可重复调用，并能 abort 正在执行的原生命令；
- context 取消会触发 abort，并同时保留 NFCX 与 Go context 错误语义；
- mock Reader 可模拟无卡、超时、设备断开和自定义操作；
- mock native Reader 连续打开和关闭 100 次后没有残留句柄。

仓库扫描确认只有 `internal/nfc/libnfc/native_cgo.go` 包含 `import "C"`。

## 未执行的硬件验收

本次自动化验收没有擅自选择串口或访问真实卡片。PN532 + FT232RL 的 Go binding 硬件验收需要明确的本机 connstring，并在读卡器上放置用户拥有的 ISO14443A 测试卡：

```sh
make libnfc-binding-hardware-smoke LIBNFC_DEVICE='pn532_uart:<serial-port>'
```

该命令会确认环境设备可枚举，执行 100 次 Reader 打开/关闭，并读取 UID、ATQA 和 SAK。完成该命令后，规格 03 的真实硬件验收项才能标记为已完成。
