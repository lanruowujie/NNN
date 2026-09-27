# 22. 手动命令行环境

完成内容：在“关于 NFCX”窗口增加与“诊断”并列的“打开命令行环境”按钮。该按钮通过 Go application service 调用独立的平台终端启动器；不会让前端拼接或执行任意命令。启动前会拒绝正在运行的 NFCX 任务，并关闭当前读卡器和后台轮询。新终端只在本次会话中将 NFCX 私有 runtime 目录前置到 `PATH`，并在已有选择时设置 `LIBNFC_DEVICE`；不会修改系统 PATH、shell 配置或全局 libnfc 配置。终端会显示已知密钥的 MFOC dump、MFCUK Darkside 和含种子密钥的 Hardnested 的常用示例，以及 `nfc-mfsetuid` 的 block 0 高风险提示。macOS 使用账户配置的登录 shell，Linux 使用账户记录中的 shell；仅在无法取得时才回退到系统兼容 shell。

平台实现：macOS 使用 AppleScript 启动 Terminal，首次使用可能需要用户授权 NFCX 控制 Terminal；Windows 使用独立 PowerShell 控制台；Linux 优先使用 `xdg-terminal-exec`，并回退至常见终端模拟器。Linux 找不到受支持终端时会返回明确错误。

问题：Linux 没有所有桌面环境通用的终端启动 API，故保留有序回退而非假定某个终端一定存在。

阻塞或未完成事项：无。用户在关闭手动终端后需要自行回到 NFCX 重新连接读卡器；这是为了避免 GUI 与外部命令同时占用同一设备。
