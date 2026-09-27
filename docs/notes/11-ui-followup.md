# 规格 07/10A/11 GUI 反馈修正记录

日期：2026-09-13

## 实际完成内容

- Dump 恢复弹窗中的“同时写入 UID”不再根据当前卡型或 sector 0 密钥状态提前禁用。用户可以先表达写 UID 的意图，下一步只读预检再统一检查 4-byte Classic 1K、已验证密钥和访问条件，并显示具体失败原因。
- 授权确认按钮明确使用普通按钮行为，避免表单默认提交导致偶发点击无响应。后续状态机调整为：无卡时入口禁用；Classic 卡片在场并打开授权弹窗后，确认按钮不受密钥或工作台状态限制；移卡时弹窗立即关闭。
- 弹窗中的预检、写入、ASCII 应用和密钥恢复确认等执行按钮显式设置为 `type="button"`，避免 `<form method="dialog">` 的默认提交/关闭行为与 JavaScript 点击处理竞争。
- ASCII block 编辑改为接受 0 到 16 个 ASCII 字节，短输入尾部自动填充 `00`。超过 16 字节或包含非 ASCII 字符仍会拒绝；零填充内容重新打开时会恢复填充前的可打印文本。

## 实现过程中遇到的问题

### 前端把可恢复的前置条件做成了不可点击状态

sector 0 密钥尚未同步到界面或卡片状态正在刷新时，禁用“同时写入 UID”复选框会阻止用户进入本来能够给出准确原因的后端预检。调整后，复选框始终允许表达意图，具体条件由权威后端预检；密钥恢复入口则遵循后续定义的贴卡会话状态机。

### ASCII 显示字符不能直接表示尾部零字节

工作台展示会把不可打印字节显示为点号。为了让 `NFCX + 12 个 00` 重新打开后仍显示为 `NFCX`，编辑器根据完整 Hex 内容识别“可打印 ASCII 前缀 + 全零尾部”；其他混合二进制内容仍保持 ASCII 输入框为空，避免错误转换。

## 阻塞或未完成事项

无。

## 验收命令

```sh
GOCACHE=/tmp/nfcx-go-cache go test ./...
GOCACHE=/tmp/nfcx-go-cache go vet ./...
GOCACHE=/tmp/nfcx-go-cache go test -race ./app ./internal/workbench ./internal/workflow
GOCACHE=/tmp/nfcx-go-cache make frontend-build
GOCACHE=/tmp/nfcx-go-cache make libnfc-binding-test
```
