# Windows 自动密钥恢复寻卡超时修复

日期：2026-09-14

## 实际完成内容

- 修复 PN532 UART 在 Windows 上已经选中卡片后，高频重复执行无约束寻卡可能返回 `nfc card info timeout` 的问题。
- `CardInfo` 对已保存的目标改用 libnfc 的同 UID 在场检查；目标消失或后端不支持该检查时，才退回一次普通寻卡，以便发现新卡。
- 保留“不主动 deselect”的约束，避免 PN53x `InRelease` 将 MIFARE Classic 卡置于 HALT 后短时间无法再次发现。
- 自动密钥扫描仍在每把候选密钥前核对卡片身份，但已选中目标的核对不再执行无约束寻卡，既保留换卡保护，又避开 Windows PN532 UART 的重复寻卡超时。
- 扩展显式硬件测试，连续调用 20 次 `CardInfo`，覆盖 GUI 轮询与工作流预检的实际调用方式。

## 实现过程中遇到的问题

GUI 后台轮询会保留最近一次成功识别到的卡片；此前 timeout 被有意视为“不能证明卡已移除”，因此界面仍显示卡片存在，但自动恢复的第一次身份预检会直接暴露同一个 timeout。根因不是密钥恢复引擎，而是 PN532 UART 对已选中目标重复执行普通 `InListPassiveTarget` 的行为不稳定。

libnfc 1.8.0 对 MIFARE Classic 的 `target_is_present` 使用最长 300 ms 的指定 UID 重选，并明确会清除之前的 Crypto1 认证状态，正好符合 `CardInfo` 的语义。目标在场时无需再执行无约束寻卡。

## 阻塞或未完成事项

- 无自动化测试阻塞。
- Windows PN532 UART 的最终结果需要用户用新构建包执行一次真实卡片自动密钥恢复验证。

## 验收命令

```sh
go test ./...
make libnfc-binding-test
LIBNFC_DEVICE='pn532_uart:<serial-port>' make libnfc-binding-hardware-smoke
```
