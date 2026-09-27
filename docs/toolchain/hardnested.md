# MFOC-Hardnested 工具链

NFCX 使用官方 [`nfc-tools/mfoc-hardnested`](https://github.com/nfc-tools/mfoc-hardnested) 的 0.10.9 源码，并固定精确 commit、归档 SHA-256、许可证和 libnfc 版本。固定值见 `third_party/mfoc-hardnested/hardnested.lock`。

macOS arm64 构建：

```sh
make hardnested-check
make hardnested-build
make hardnested-verify
```

脚本构建源码而不下载预编译二进制，使用 `build/toolchain/darwin-arm64/sdk` 中与 NFCX 相同的 libnfc SDK。输出位于 `runtime/darwin-arm64/mfoc-hardnested`，并附带源码锁、二进制 SHA-256 和 GPL 许可证。

离线或审计构建可传入已校验的归档：

```sh
NFCX_HARDNESTED_ARCHIVE=/absolute/path/to/archive.tar.gz make hardnested-build
```

归档仍必须与 lock 中的 SHA-256 完全一致。
