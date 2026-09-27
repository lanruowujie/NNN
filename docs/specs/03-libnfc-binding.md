# 规格 03：libnfc C shim 与 Go Reader

## 目标

让 Go 主进程通过一个窄而稳定的接口使用 libnfc，同时隔离 libnfc 的 C 数据结构、内存管理和错误码。

## 前置条件

- 规格 02 已验收。
- 当前平台能链接并加载固定版本的 libnfc。

## 范围

- 定义与硬件无关的 Go `Reader`、`DeviceInfo`、`CardInfo` 和错误类型。
- 在唯一允许 `import "C"` 的包中实现 CGO binding。
- 编写 C shim，包装初始化、设备枚举、打开、关闭、initiator 初始化和 ISO14443A 寻卡。
- 预留认证、读块、写块函数签名；具体 Classic 工作流在规格 05 完成。
- 实现 mock Reader，使上层测试不依赖 CGO。
- 明确所有 C 内存和设备句柄的所有权。

## 不在本阶段实现

- GUI 自动轮询；
- 整卡读取和写入；
- 外部进程；
- 特殊 UID 卡功能。

## 设计要求

上层接口不得暴露任何 libnfc 类型：

```go
type Reader interface {
	Open(ctx context.Context, connString string) error
	Close() error
	CardInfo(ctx context.Context) (CardInfo, error)
	Authenticate(ctx context.Context, block byte, keyType KeyType, key Key) error
	ReadBlock(ctx context.Context, block byte) ([16]byte, error)
	WriteBlock(ctx context.Context, block byte, data [16]byte) error
}
```

C shim 需要：

- 将 libnfc 句柄封装成 opaque handle；
- 对所有输出 buffer 携带长度；
- 不返回指向栈内存的指针；
- 为“无卡”“超时”“认证失败”“设备断开”和一般 I/O 错误提供稳定错误码；
- 关闭操作可重复调用；
- 打开失败时不泄漏 context 或 device；
- 阻塞调用能够被上层通过超时或 abort 机制结束。

同一个底层句柄不得从多个 goroutine 并发调用。并发串行化由 Go wrapper 或后续 DeviceManager 明确负责。

## 自动化测试

- C 错误码到 Go 错误的完整映射；
- `Close` 的幂等性；
- 打开失败后的资源清理；
- UID 4/7/10 字节能够安全复制到 Go；
- mock Reader 可模拟无卡、超时和设备拔出；
- 启用 CGO 的 smoke test 能初始化并退出 libnfc。

## 验收标准

- [ ] 只有指定封装包包含 `import "C"`。
- [ ] application service 和 GUI 不包含 libnfc 类型。
- [ ] Go 可以列出设备、打开设备并关闭设备。
- [ ] 有卡时可得到 UID、ATQA、SAK；无卡时返回可判定错误。
- [ ] 连续打开/关闭 100 次无明显句柄或内存泄漏。
- [ ] mock 测试可在没有 libnfc 设备的 CI 中运行。
- [ ] `go test ./...` 通过。

