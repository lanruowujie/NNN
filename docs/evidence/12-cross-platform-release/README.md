# 规格 12 验收证据

本目录用于保存 GitHub Actions 发布运行链接、最终 SHA-256、签名/公证验证输出，
以及三台干净机器的手工验收记录。

当前自动化实现已加入仓库；以下证据必须在首次候选版本运行后补充：

- GitHub Actions `Cross-platform release` 成功运行链接；
- macOS `codesign`、`notarytool`、`stapler` 和 Gatekeeper 验证输出；
- Windows 签名状态及干净环境启动记录；
- Linux AppImage/tar.gz 执行权限和干净环境启动记录；
- 三个平台 `--self-check --json` 输出；
- PN532 UART 手工读写以及 mfoc 取消/设备恢复记录。

不得在这里提交真实卡片密钥、未脱敏 connstring、用户名或签名凭据。
