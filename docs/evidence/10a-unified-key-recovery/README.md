# 规格 10A 验收记录

## 自动化

```sh
GOCACHE=/tmp/nfcx-go-cache go test ./...
GOCACHE=/tmp/nfcx-go-cache go vet ./...
GOCACHE=/tmp/nfcx-go-cache go test -race ./app ./internal/attack ./internal/device ./internal/keys ./internal/workbench ./internal/workflow
GOCACHE=/tmp/nfcx-go-cache wails generate module
cd frontend && npm run build
make build
```

## 手工硬件验收

- [ ] 连接 PN532 UART，放入经授权的 MIFARE Classic 1K。
- [ ] `自动恢复密钥` 按钮可用，确认弹窗显示 Darkside 最长 1 小时与 Hardnested 最长 6 小时。
- [ ] 常见密钥扫描结果为零时自动进入第 2 步，进度条保持动态且日志持续显示心跳或尝试次数。
- [ ] 恢复出种子后自动进入第 3 步，且第 2 步已显示完成。
- [ ] 最终弹窗准确显示 complete、partial 或 failed；失败包含步骤与原因。
- [ ] complete 时工作台为完整 dump，`读取整卡` 按钮可用。
- [ ] 取消后进程退出、设备恢复且没有错误的失败弹窗。

请在此文件下方追加测试时间、应用版本、卡片类别、每步结果和必要截图；不要记录真实完整密钥。
