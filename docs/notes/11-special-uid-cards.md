# 规格 11 实施记录：写入特殊卡 UID

日期：2026-09-13

## 实际完成内容

- 将需求收敛为 MIFARE Classic 1K、4-byte UID。优先使用 CUID/Gen2 普通认证写 block 0；若权威重选确认旧 UID 未改变，则通过外部 `nfc-mfsetuid` 适配器尝试一次 Gen1A 写入。FUID/UFUID、长 UID、Classic 4K 和锁定命令均未实现。
- 在 `Reader` 之外新增独立 `ClassicManufacturerWriter` 能力。普通 `WriteBlock` 继续拒绝 block 0；libnfc C shim 只有在 sector 0 已认证时才允许专用接口发送 block 0 写命令。
- 新增统一 UID workflow：验证卡片和 sector 0 密钥、读取访问位与当前 block 0、核对旧 UID/BCC、生成新 UID/BCC、持久化备份、执行写入、重新寻卡，并重新认证后完整回读 block 0。
- 写入内容只修改 bytes 0..4，目标卡原有 manufacturer bytes 5..15 原样保留。普通写命令结束但旧 UID 未改变时进入一次 Gen1A fallback；出现其他 UID、卡型变化、重新认证失败或完整回读不一致都会报告失败。
- 备份保存为 NFCX 用户配置目录下 `uid-backups` 中的 JSON 文件，包含卡片、设备、旧/新 block 0 和时间。目录权限为 `0700`，文件权限为 `0600`，使用临时文件同步后原子改名，文件名带唯一后缀避免覆盖。
- 应用层增加一次性、五分钟有效的预检 token 和目标 UID 二次输入确认。输入接受连续十六进制、空格、冒号或短横线格式，但必须恰好为 4 字节。
- UID 成功改变后，将旧 UID 卡片标识下的全部已验证密钥关联复制到新 UID 卡片标识，避免用户必须重新扫描所有 sector。
- “恢复整卡”重命名为“恢复备份”。默认仍不写 block 0；恢复选项可勾选“同时写入 UID”，默认采用 Dump 元数据中的 4-byte UID 并允许编辑。组合流程先恢复并验证 block 1 之后的数据与 sector trailer，最后才写 UID。
- 组合预检除了检查当前卡片的 block 0 写权限，还检查 Dump 恢复后 sector 0 的访问位及可验证新密钥，避免恢复到一半才发现 UID 无法写入。
- 卡片摘要增加独立“修改 UID”入口。两个 UI 入口展示旧/新 block 0、BCC 和风险，要求再次输入目标 UID；任务日志展示结果、实际回读和备份路径。
- 删除 GUI 区块和弹窗标题上方的装饰性英文副标题，包括 `READER`、`CARD`、`AUTH MAP`、`MEMORY IMAGE`、`ACTIVITY`、`OPERATIONS` 等，为后续 i18n 保留正常中文标题。
- 修复 PN532/libnfc 在 block 0 改变身份时把最终写入响应报告为 RF/I/O 错误并提前终止的问题。专用 manufacturer 写入现在使用 1 秒响应超时，写后主动关闭并重新开启 RF 场；RF、超时、无卡或身份变化类返回被视为“提交状态未知”，继续以重新寻卡、新 UID、BCC 和完整 block 0 回读作为唯一成功依据。底层错误文本会在再次探测卡片前保存，不再出现误导性的 `nfc write manufacturer block: Success`。
- 现场备份链证明 `4A BC 36 5E → 4A BC 36 55 → 4A BC 36 56` 的普通写路径曾连续成功，而另一张 `E3 27 8A 8E` 卡在 RF 重置修复之前就多次拒绝普通写。因此问题不是 UID/BCC 回归，而是同类外观卡片包含不同 magic generation。
- 增加 Gen1A fallback：普通写后明确仍检测到旧 UID 时，DeviceManager 停止轮询并关闭进程内 libnfc handle，再运行随固定 libnfc 1.8.0 构建并分发的 `nfc-mfsetuid <完整 block0>`。工具结束后重新打开 reader，并由 NFCX 自己验证新 UID、BCC 和完整 block 0；工具自身即使退出码为零也不直接视为成功。
- 保留 `nfc-mfsetuid` 的逐帧输出，并将两步 Gen1A 解锁结果汇总到普通操作日志。这样可区分“普通 CUID/Gen2 写入被拒绝”“Gen1A 后门未响应”和“后门解锁成功但写后校验失败”，不再只显示笼统的目标 UID 未出现。
- toolchain、运行时打包、签名、源版本记录和 BSD-2-Clause 许可证清单同步加入 `nfc-mfsetuid`。

## 实现过程中遇到的问题

### 修改 UID 后不能在原选择会话中直接回读

认证命令和 libnfc 选择状态仍绑定旧 UID。专用底层写接口因此只表示写命令已被卡片接受，不做普通 `WriteBlock` 的同会话回读；workflow 会清除旧选择、反复重新寻卡，并只在新 UID 出现后重新认证和读取完整 block 0。若旧 UID 始终存在，才允许进入 Gen1A fallback；卡片消失或出现第三个 UID 时不会发送后门写入。

现场日志进一步表明，改变 block 0 时 PN532 可能在卡片身份切换点丢失最后一个响应；原实现随后调用存在性探测，覆盖了 libnfc 的原始错误文本，于是显示成错误的 `Success`，并在有机会验证实际写入结果前中止。现在写入后强制 RF 场复位；即使最终响应不确定，也继续执行权威回读验证。若卡片实际不支持普通 CUID/Gen2 写入，新 UID 不会出现，流程仍明确失败，不会把传输异常误报为成功。

对 `E3 27 8A 8E` 卡执行了同卡、同目标 block 0 的两条独立硬件诊断。仓库既有 `cuid.py` 能完成选卡、全 F Key A 认证和 block 0 读取，但普通 A0 写入收到 PN532 `D5 41 01` 并失败；固定版本 `nfc-mfsetuid` 则在第一步 7-bit `0x40` 解锁命令就未获响应。该卡因此同时不具备当前支持的 CUID/Gen2 普通写能力和 Gen1A 后门能力；结合“曾经只能修改一次”的现象，高度疑似一次写入后已锁定的 FUID/UFUID，但仅凭这些响应不能断言具体代际。

### Dump 恢复会改变后续 UID 写入条件

整卡恢复会在最后替换 sector 0 trailer，当前卡的写权限和密钥不一定仍适用。组合预检现在同时解析 Dump 中即将生效的访问位；执行 UID 步骤前，再使用 restore 已实际验证并登记的新 sector 0 密钥完整重跑一次 UID 预检。

### 原始 Dump 不一定携带可靠 UID 长度

没有 NFCX sidecar 的 raw Dump 无法仅凭 block 0 稳妥区分 4-byte 与级联 UID 布局，因此界面只在元数据明确记录 4-byte UID 时自动填入。没有可靠元数据时保留输入框，由用户明确输入，不从 raw bytes 猜测。

## 阻塞或未完成事项

- 自动化测试、前端构建和 libnfc CGO shim 编译无阻塞。
- 当前 `E3 27 8A 8E` 卡已完成真实 PN532 + FT232RL 对照测试，确认拒绝普通写和 Gen1A 解锁；NFCX 未实现 FUID/UFUID 或带私有密码的 Gen3/Gen4 配置命令，也不会对未知卡片盲发不可逆锁定命令。

## 验收结论

- 2026-09-13：用户确认验收通过。
- CUID/Gen2 普通认证写、旧 UID 未改变时的 Gen1A fallback、写前备份、BCC 计算、重新寻卡及完整 block 0 回读验证均按本规格交付。
- 对不支持普通 block 0 写入且不响应 Gen1A 解锁的卡片，应用会保持原 UID、报告失败并输出协议阶段诊断；`E3 27 8A 8E` 实卡表现已归入这一预期失败分支，不再作为 NFCX 写入回归问题。

## 验收命令

```sh
GOCACHE=/tmp/nfcx-go-cache go test ./...
GOCACHE=/tmp/nfcx-go-cache go vet ./...
GOCACHE=/tmp/nfcx-go-cache go test -race ./app ./internal/attack ./internal/device ./internal/keys ./internal/mifare ./internal/nfc ./internal/nfc/libnfc ./internal/workbench ./internal/workflow
GOCACHE=/tmp/nfcx-go-cache make frontend-build
GOCACHE=/tmp/nfcx-go-cache make libnfc-binding-test
```
