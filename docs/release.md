# NFCX 跨平台发布

首批发布目标为 macOS arm64、Windows amd64 和 Linux amd64。构建及发布只通过
GitHub Actions 的原生 runner 执行，不从 Linux 交叉编译其他平台。

## 产物

- `NFCX-<version>-darwin-arm64.dmg`：配置 Apple 发布凭据时使用 Developer ID 签名、
  公证并 staple；未配置时生成 `NFCX-<version>-darwin-arm64-unsigned.dmg`，使用 ad-hoc
  签名且不公证；
- `NFCX-<version>-windows-amd64.zip`：有证书时签名；缺少证书的手工构建增加
  `-unsigned` 后缀；
- `NFCX-<version>-linux-amd64.AppImage`；
- `NFCX-<version>-linux-amd64.tar.gz`；
- `SHA256SUMS`。

macOS DMG 打开后包含 `NFCX.app`、指向系统 `/Applications` 的 `Applications` 目录链接，
以及简短安装提示；将应用拖到该目录即可安装。

首版正式支持 PN532 UART，包括 PN532 + FT232RL。其他 libnfc 读卡器驱动在完成
对应平台硬件验收后再加入支持矩阵。

## GitHub Actions

`.github/workflows/check.yml` 在三个目标系统上运行 Go 测试、vet、Wails binding
生成和前端生产构建。

`.github/workflows/release.yml` 可手工运行，也会响应 `v<semver>` 标签。手工运行
生成测试产物；标签构建在三个平台全部成功后创建或更新 GitHub Release。

macOS 签名与公证是可选的。以下六个 repository secrets 必须全部配置或全部留空：

- `MACOS_CERTIFICATE`：Developer ID Application `.p12` 的 base64；
- `MACOS_CERTIFICATE_PASSWORD`；
- `MACOS_SIGNING_IDENTITY`；
- `APPLE_ID`；
- `APPLE_APP_PASSWORD`：app-specific password；
- `APPLE_TEAM_ID`。

未配置这些 secrets 时，手工与标签发布仍会成功，但产物带 `-unsigned` 后缀。该产物
可用于自行测试和分发给明确知情的用户；由于没有 Apple Developer ID 和公证，其他
Mac 上首次打开时会出现 Gatekeeper 警告。获得系统认可的 Developer ID 签名与公证
不能由 CI 绕过，需要有效的 Apple Developer Program 资格。

Windows 签名是可选的：

- `WINDOWS_CERTIFICATE`：代码签名 `.pfx` 的 base64；
- `WINDOWS_CERTIFICATE_PASSWORD`。

任何密钥都不会写入仓库或构建产物。

## Runtime 与完整性

运行时从仓库锁定的源码和 SHA-256 构建。应用不搜索用户提供的工具目录，也不
修改 `/etc/nfc` 或其他系统级 libnfc 配置。

macOS 和 Linux 将外部工具放在应用的 `runtime/<os>-<arch>` 目录；Windows 因
DLL 必须在 Go 代码运行前由系统加载，portable ZIP 使用应用根目录作为私有
runtime。三种布局都包含 `manifest.json`，列出组件来源、版本、commit、许可证
及逐文件 SHA-256。

发布构建还会聚合实际链接到应用的 Go module 许可证。GPL 工具的 source lock、
源码 URL、源码 SHA-256 和 NFCX patch 一并包含在产物中，作为对应源码获取说明。

## 无硬件自检

最终解压目录可运行：

```text
NFCX --self-check --json
```

该命令不会打开 GUI 或 NFC 设备。它检查：

- NFCX/commit/Go/Wails/OS/architecture 构建信息；
- 私有 libnfc 的加载、版本和启用驱动；
- mfoc、mfcuk、mfoc-hardnested、nfc-mfsetuid 的版本/能力；
- runtime manifest 和每个文件的 SHA-256。

同一份报告显示在 GUI 的“关于 / 诊断”页面中。涉及用户主目录的错误路径会脱敏。

## 手工验收

每个平台必须在没有 Go、Node、Wails、libnfc、mfoc 或 mfcuk 的干净环境执行：

1. 安装或解压并启动；
2. 检查“关于 / 诊断”全部通过；
3. 连接 PN532 UART；
4. 寻卡并读取授权测试卡；
5. 保存 1024-byte MIFARE Classic 1K dump；
6. 在授权测试卡执行一次非 block 0 写入并回读验证；
7. 至少在一个平台启动并取消 mfoc，确认设备被关闭后重新打开。

结果记录在 `docs/evidence/12-cross-platform-release/`，真实硬件测试不得放入普通
CI。
