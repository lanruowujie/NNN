# 规格 10B 验收记录

## 自动化

```sh
NFCX_HARDNESTED_ARCHIVE=/tmp/nfcx-mfoc-hardnested-a600743.tar.gz make hardnested-build
make hardnested-verify
GOCACHE=/tmp/nfcx-go-cache go test ./...
GOCACHE=/tmp/nfcx-go-cache go vet ./...
GOCACHE=/tmp/nfcx-go-cache go test -race ./app ./internal/attack ./internal/device ./internal/keys ./internal/workbench ./internal/workflow
GOCACHE=/tmp/nfcx-go-cache make libnfc-binding-test
make build
codesign --verify --deep --strict build/bin/NFCX.app
```

## 手工硬件验收

- [ ] 连接 PN532 UART，设备摘要显示 Hardnested。
- [ ] 放入经授权的 hardened nonce Classic 1K，并确保至少有一把已验证种子密钥。
- [ ] Nested 失败或覆盖不完整后自动进入第 4/5 步。
- [ ] 第 4 步显示动态进度、已用时、6 小时上限和可取消状态。
- [ ] Hardnested 输出成功后，NFCX 用 libnfc 逐项复验候选密钥。
- [ ] 第 5 步实际读取，结果弹窗显示正确覆盖数和可读扇区数。
- [ ] 取消或失败后 reader 恢复，已有已验证密钥仍保留。

请追加测试时间、应用版本、卡片类别、各步骤结果和必要截图；不要记录真实完整密钥。
