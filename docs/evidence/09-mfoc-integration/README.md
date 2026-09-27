# 规格 09 自动化验收证据：MFOC Nested 集成

日期：2026-09-13  
平台：macOS arm64  
Go：1.25.1  
Wails：2.15.0  
libnfc：1.8.0  
MFOC：0.10.7

## 已通过检查

```sh
GOCACHE=/tmp/nfcx-go-cache go test ./...
GOCACHE=/tmp/nfcx-go-cache go vet ./...
GOCACHE=/tmp/nfcx-go-cache go test -race \
  ./app ./internal/attack ./internal/device ./internal/keys \
  ./internal/workbench ./internal/workflow
NFCX_MFOC_ARCHIVE=/tmp/mfoc-0.10.7.tar.gz ./scripts/toolchain/mfoc.sh build
./scripts/toolchain/mfoc.sh verify
GOCACHE=/tmp/nfcx-go-cache wails generate module
cd frontend && npm run build
```

## 自动化覆盖

- 只有 `pn532_uart` profile 默认开放 Nested，未知和其他驱动保持关闭；
- 必须提供授权确认和至少一条属于当前卡片的已验证 Key A 或 Key B；
- 关闭 reader 前在同一设备锁内重选完整卡片身份，换卡时拒绝启动并恢复 reader；
- 重复 `-k <key>` 与 `-O` 参数、带空格路径、Key A/Key B 种子选择及重复值去重；
- 固定程序路径、动态依赖可启动性和严格的 0.10.7 版本检查；
- 成功、非零退出、timeout、缺失输出和损坏输出；
- MFOC 0.10.7 的 1K-in-4K 全零填充输出、非零尾部拒绝、UID、BCC、访问位和 32 个 trailer 密钥候选提取；
- stdout/stderr 任意分片下的 sector 进度解析与跨 chunk 密钥脱敏；
- 每个候选按正确 sector 和 Key A/Key B 使用 mock reader 重新认证；
- 认证失败降级为 rejected，不进入已验证集合；
- 已有不同密钥时原子报告 conflict，不覆盖旧验证、不污染全局字典；
- MFOC artifact 不直接进入工作台，libnfc 重新读取结果只填补未知数据；
- 已知字节、用户编辑和撤销历史在合并时保持，冲突可计数；
- Wails DTO 只输出计数和状态，不序列化候选密钥值；
- 前端 TypeScript 类型检查与生产构建。

所有自动化 MFOC 进程测试使用 `cmd/nfcx-fake-mfoc`，不会接触真实读卡器、卡片或用户密钥。

## 运行时证据

已从锁定源码构建 `runtime/darwin-arm64/mfoc`，验证：

- `mfoc -h` 报告 0.10.7；
- 动态依赖为 `@rpath/libnfc.6.dylib` 和 macOS `libSystem`；
- `LC_RPATH` 为 `@executable_path`；
- 运行时目录包含 MFOC GPL、libnfc LGPL、两份来源锁和生成的二进制 SHA-256；
- 运行时复制进 `.app` 后，嵌套 dylib、MFOC 和整个应用依次完成本地签名并通过 `codesign --verify --deep --strict`。

## 手工硬件验收状态

2026-09-13，用户使用 PN532 + FT232RL 与自有 MIFARE Classic 1K 测试卡确认验收通过：

- 默认全 F 卡完成 MFOC 快速路径、设备交接、输出和整卡读取验证；
- 随机密钥测试卡只提供一个已知 Key A 值，默认扫描后 Key B 仍未知；
- MFOC 明确选择 sector 0 作为 exploit sector 并进入 Nested，恢复未知 Key B，随后报告所有扇区认证成功；
- 首轮真实测试发现并修正了 MFOC 0.10.7 使用重复 `-k` 而非 `-f` 的 CLI 差异；
- 第二轮发现并修正了该版本对 Classic 1K 仍写出 4096 字节固定结构的兼容差异；
- 修正后，NFCX 接受经过严格尾部校验的输出，通过 in-process libnfc 复验恢复密钥，并可正常读取卡片；
- 用户最终确认：只要存在一个经验证的有效密钥，当前 MFOC Nested 工作流即可启动并完成恢复。

普通 `go test ./...` 仍不会自动访问真实读卡器、运行破解或写卡。
