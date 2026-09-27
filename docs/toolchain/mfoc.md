# MFOC 运行时工具链

## 固定来源

- 上游：<https://github.com/nfc-tools/mfoc>
- tag：`mfoc-0.10.7`
- commit：`290a0759567d4df4840d9133d663ac88246b373a`
- 源码包：<https://github.com/nfc-tools/mfoc/archive/refs/tags/mfoc-0.10.7.tar.gz>
- 源码 SHA-256：`2dfd8ffa4a8b357807680d190a91c8cf3db54b4211a781edc1108af401dbaad7`
- 许可证：GPL-2.0-or-later；运行时分发包含上游 `COPYING` 和 NFCX 源码补丁

机器可读值位于 `third_party/mfoc/mfoc.lock`。构建脚本在解包前校验源码 SHA-256，不跟踪上游最新提交。

## 构建关系

MFOC 使用 `scripts/toolchain/mfoc.sh` 构建，并通过 `PKG_CONFIG_PATH` 链接 NFCX 已固定的 patched libnfc 1.8.0 SDK。macOS arm64 产物依赖：

```text
@rpath/libnfc.6.dylib
/usr/lib/libSystem.B.dylib
```

脚本为 MFOC 增加 `@executable_path` rpath，使它在应用的同一平台 runtime 目录内加载打包的 libnfc，而不依赖 Homebrew 或系统安装。构建会应用锁定的最小兼容补丁：将 `usage` 函数的 `errno` 形参重命名为 `exit_code`，避免现代 MinGW-w64 的 `errno` accessor 宏破坏函数签名；补丁文件随 runtime 一并分发。

```sh
make mfoc-check
make mfoc-build
make mfoc-verify
```

生成文件位于 `runtime/darwin-arm64`，不会提交到 Git。`make build` 会将 MFOC、libnfc、来源锁、二进制哈希和许可证复制进应用资源目录，先签名嵌套二进制，再重新签名并校验应用。

## 运行时检测

NFCX 只从受控 `runtime/<os>-<arch>` 路径解析固定文件名 `mfoc` 或 `mfoc.exe`。可用性检测会实际运行 `mfoc -h`：动态库缺失、进程不能启动、版本输出不识别、版本不是 0.10.7，或帮助中缺少可重复的 `-k <key>` 选项，均视为不可用。已验证种子按值去重后分别通过 `-k` 传入；该固定版本不支持 `-f` 字典参数。

运行任务时不会修改系统 libnfc 配置。精确 connstring 只通过子进程环境 `LIBNFC_DEVICE` 传递，同时关闭自动和侵入式扫描。
