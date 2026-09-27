# Windows PN532 UART 自动发现修复

## 实际完成

- 确认显式 `LIBNFC_DEVICE=pn532_uart:COM3:115200` 可以发现设备，将故障范围缩小到自动发现配置。
- 确认在进程启动前显式设置自动扫描和侵入式扫描仍无法发现 COM3，进一步定位到 libnfc 1.8.0 的 Windows UART 端口候选枚举。
- 在 C shim 中增加 Windows 发现环境同步：若 C 运行库尚未包含对应变量，则在 `nfc_init()` 前设置 `LIBNFC_AUTO_SCAN=true` 与 `LIBNFC_INTRUSIVE_SCAN=true`。
- 保留进程启动前由用户显式传入的环境值，不覆盖 `false` 等调试配置。
- 增加固定版本 libnfc 补丁，使用 `QueryDosDeviceA` 查询当前 COM 设备映射，替换可能遗漏 USB 串口的 `GetDefaultCommConfig` 探测。
- 增加 CGO 回归测试，验证发现默认值能够在 C 运行库中完成配置。
- 更新 libnfc 工具链文档，记录 Windows 上 Go 环境与 C 运行库环境不同步的原因和边界。

## 遇到的问题

Windows 上存在两个连续问题：

1. Go 的 `os.Setenv` 更新 Win32 进程环境，但 libnfc 使用 C 运行库的 `getenv()` 读取设置，应用启动后写入的扫描默认值不能保证同步。
2. 即使从进程启动前显式启用扫描，libnfc 1.8.0 仍通过 `GetDefaultCommConfig` 决定 COM 端口是否存在；实测 COM3 可以用精确 connstring 打开，却没有进入自动扫描候选集。

## 阻塞或未完成

需要由 Windows GitHub Actions 重新构建发布包，并在真实 PN532 + FT232RL 的 COM 端口上确认无需外部环境变量即可自动发现。其余阻塞：无。
