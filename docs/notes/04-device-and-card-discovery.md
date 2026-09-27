# 规格 04 实施记录：设备管理与寻卡

日期：2026-09-12

## 实际完成内容

- 在 `internal/device` 实现唯一拥有选中 Reader 的 `DeviceManager`，覆盖 `disconnected`、`connecting`、`ready`、`polling`、`busy`、`recovering` 和 `error` 状态。
- 使用可取消的后台循环持续选择 ISO/IEC 14443A 卡片；每次操作有独立超时，但不会重复初始化同一连接的 libnfc context。
- 通过设备操作门串行化轮询和独占操作，为后续认证、读写及外部任务切换预留统一入口。
- 使用 UID、UID 长度、ATQA 和 SAK 判断卡片是否相同；相同卡片只更新内部最后检测时间，不重复发送事件。
- 实现卡片移除防抖和更换顺序：更换时先发送旧卡移除，再发送新卡出现。
- 对设备断开、I/O 和打开超时执行有限退避重连；权限、参数和不支持错误不会无限重试。
- 增加稳定的权限错误类别；在 macOS/Linux 的 PN532 UART 打开失败路径中通过系统权限检查区分已存在但不可读写的串口。
- 保留手动 connstring 的原始字节，不裁剪、不规范化，也不写入配置或日志。
- C shim 新增打开后设备名称复制接口，GUI 可以同时显示 libnfc 设备名称和 connstring，且不会暴露 native 指针。
- Wails application service 新增设备刷新、连接和断开 binding，以及结构化设备/卡片事件 DTO。
- GUI 已移除模拟设备、模拟卡片和模拟扇区内容，改为显示真实连接状态、UID、UID 长度、ATQA、SAK 和保守卡型推断。
- `make dev` 与 `make build` 现在显式链接规格 02 的固定 libnfc SDK；普通测试仍使用无硬件 stub。
- 修复连接按钮状态：设备进入 `ready`、`polling`、`busy` 或 `recovering` 后，同一按钮切换为“断开”；断开后再切回“连接”，连接过程中不可重复点击。
- 修复 PN532 UART 自动发现：NFCX 不读取系统 `libnfc.conf`，因此在应用进程内为未显式设置的 `LIBNFC_AUTO_SCAN` 和 `LIBNFC_INTRUSIVE_SCAN` 提供 `true` 默认值，使 libnfc 实际探测串口；用户环境中的显式值仍优先。

## 实现过程中遇到的问题

### 枚举阶段无法取得友好名称

`nfc_list_devices` 可靠返回 connstring，但设备友好名称需要打开句柄后调用 `nfc_device_get_name`。实现没有为了名称在枚举时逐个打开设备，而是在用户连接成功后通过窄 C shim 复制名称并更新 GUI。

### libnfc 打开失败缺少权限细分类

部分 libnfc 驱动在 `nfc_open` 返回空指针时不会保留可映射的 errno。NFCX 只在 macOS/Linux、准确的 `pn532_uart:` connstring、且 libnfc 已报告设备无法打开时检查设备节点读写权限；不解析自然语言错误，也不修改系统权限。

### 轮询超时与卡片移除不同

单次超时可能来自设备延迟，不能证明卡片已经移除。因此超时只继续轮询；只有连续两次明确的“无卡”结果才产生移除事件。

### 系统 nfc-list 与 NFCX 枚举结果不同

Homebrew 的 `nfc-list` 会读取 `libnfc.conf`，当前机器的配置启用了 `allow_intrusive_scan=yes`。NFCX 固定版按设计关闭配置文件读取，初始实现没有补上等价的进程内默认值，导致 `nfc_list_devices` 不执行 PN532 UART 串口探测。现在由应用启动代码在创建 backend 前设置缺失的环境选项，不修改系统配置，也不覆盖用户显式选择。

## 阻塞或未完成事项

- 自动化测试与 CGO 编译测试已完成。
- 用户已在 2026-09-12 确认规格 04 的真实设备验收完成：PN532 + FT232RL 连接后，放置测试卡可以读取卡片信息。本轮规格 05 实施没有重复修改该 GUI 流程。
- MIFARE Classic 认证、块读取和块写入属于规格 05，当前 GUI 中相应操作保持禁用。
- 将 libnfc 动态库嵌入应用包、签名和发布安装包属于规格 12；本阶段通过 `make dev` 执行真实设备 GUI 验收。

## 验收命令

```sh
go test ./...
go test -race ./internal/device/... ./internal/nfc/... ./app/...
go vet ./...
make frontend-build
make libnfc-binding-test
```

手工硬件验收：

```sh
make dev
```

在 GUI 中选择自动发现的设备，或输入 `pn532_uart:<serial-port>`，然后执行规格 04 的插卡、移卡、换卡和读卡器重新插拔步骤。
