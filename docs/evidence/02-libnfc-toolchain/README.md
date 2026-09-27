# 规格 02 验收证据

日期：2026-09-12

## 环境

- macOS 14.7.8 (`Darwin 23.6.0`)
- Apple Silicon arm64
- Apple clang 16.0.0
- GNU Make 3.81
- libnfc 1.8.0 release archive
- NFCX 驱动配置：`pn532_uart`

## 自动化结果

### 来源校验

官方 release archive 的 SHA-256 与 `third_party/libnfc/libnfc.lock` 一致：

```text
6d9ad31c86408711f0a60f05b1933101c7497683c2e0d8917d1611a3feba3dd5
```

### 构建与产物验证

`make libnfc-build` 和 `make libnfc-verify` 通过。验证结果：

```text
libnfc verify: PASS
  driver: pn532_uart only
  architecture: arm64
  install name: @rpath/libnfc.6.dylib
  private dependencies: none
```

`otool -L` 显示运行时只有自身 `@rpath` ID 和 macOS 系统库依赖：

```text
@rpath/libnfc.6.dylib
/usr/lib/libSystem.B.dylib
```

### 最小 C smoke test

`make libnfc-smoke` 通过：

```text
libnfc smoke: version=1.8.0 devices=0
libnfc smoke: version=1.8.0 devices=1
```

测试显式关闭自动扫描和侵入式扫描。第一轮在无设备时正常返回零设备；第二轮注入一个不会实际打开的 synthetic `pn532_uart` connstring，并确认显式设备能够被列出。这覆盖了禁用 conffiles 后 `LIBNFC_DEVICE` 仍然生效的回归场景。

### 干净目录重复构建

`make libnfc-repeatability-check` 在两个相互独立的临时目录中分别执行完整构建、产物检查和 C smoke test，两轮均通过：

```text
libnfc repeatability: PASS (two independent clean builds)
```

### 项目回归

`make check` 通过，包括 Go tests、Go vet、Wails binding 生成和前端生产构建。

## 真实硬件验收

使用 PN532 + FT232RL 和显式串口完成只读验收：

```sh
LIBNFC_LOG_LEVEL=3 make libnfc-hardware-smoke \
  LIBNFC_DEVICE='pn532_uart:/dev/cu.usbserial-A50285BI'
```

结果：

```text
NFC device: user defined device opened
0 ISO14443A passive target(s) found.
```

调试日志同时确认：

- 串口以 115200 baud 打开；
- PN532 对 `SAMConfiguration`、`GetFirmwareVersion` 及后续命令均返回 ACK；
- 命令正常退出，设备在结束时完成 RF field 关闭和 `PowerDown`；
- 测试时天线范围内没有卡，因此没有 UID、ATQA、SAK 可记录。这不是读卡器连接失败。

仍待放置测试卡后的卡片信息验收，以及物理拔插后的再次识别。

通用验收命令：

```sh
make libnfc-hardware-smoke LIBNFC_DEVICE='pn532_uart:<serial-port>'
```
