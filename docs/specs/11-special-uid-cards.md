# 规格 11：写入特殊卡 UID

## 目标

本阶段只解决一个明确需求：让用户能够安全地写入 MIFARE Classic 1K 特殊卡的 4-byte UID。入口同时出现在 Dump 恢复流程和卡片摘要区域，但两处共用同一套 block 0 写入、备份与验证工作流。实现先尝试 CUID/Gen2 普通认证写；旧 UID 未改变时，再释放进程内 reader 并使用固定版 `nfc-mfsetuid` 尝试 Gen1A 后门序列。

## 产品定义

- 原“恢复整卡”改名为“恢复备份”。它恢复备份中 block 1 之后的数据块和 sector trailer，并逐块验证；默认始终跳过 block 0，因此默认不会修改 UID。
- 恢复 Dump 时可随时勾选“同时写入 UID”，不因尚未验证 sector 0 密钥而提前禁用。目标 UID 默认取 NFCX Dump 元数据中保存的 4-byte UID，用户可以修改；原始 Dump 没有可靠 UID 长度元数据时不猜测，要求用户输入。卡型、UID 长度、密钥和访问条件统一由下一步只读预检判断。
- 卡片摘要提供独立“修改 UID”入口，只写 block 0 中的 UID/BCC，不恢复其他数据。
- 两个入口都只支持 4-byte UID 和 MIFARE Classic 1K 特殊卡；当前覆盖 CUID/Gen2 普通认证写与 PN532 UART 上的 Gen1A 写入。
- 去除各区块标题上方的装饰性英文副标题，例如 `READER`、`CARD`、`AUTH MAP`、`MEMORY IMAGE`。技术名词和协议名不在本次 i18n 调整范围内。

## 本阶段范围

- 通过 in-process libnfc，在 sector 0 认证后向 block 0 发送普通 MIFARE Write 命令。
- 普通写确认旧 UID 未改变后，通过统一设备所有权交接调用同一固定 libnfc 1.8.0 构建产物中的 `nfc-mfsetuid`，尝试 Gen1A 解锁和完整 block 0 写入。
- 计算并验证 4-byte UID 的 BCC。
- 只替换 block 0 的 bytes 0..4，保留目标卡当前的 manufacturer bytes 5..15。
- 写入前显示旧、新完整 block 0，并要求再次输入目标 UID 确认。
- 写入前将目标卡原始 block 0 持久化到 NFCX 配置目录下的 `uid-backups`。
- 写命令后重新寻卡，验证新 UID、ATQA、SAK、BCC 和完整 block 0。
- UID 改变后，将该卡已有的已验证密钥关联迁移到新卡片标识。
- Dump 恢复与 UID 写入组合时，先完成普通数据和 sector trailer 恢复，最后写 UID；预检同时检查当前访问条件和恢复后的 sector 0 访问条件。

## 不在本阶段实现

- 预先猜测 magic card generation；实现只依据普通写后的实际 UID 是否改变决定是否尝试 Gen1A 路径；
- 7-byte 或 10-byte UID 写入；
- Classic 4K 的 UID 写入；
- FUID/UFUID 锁定以及 Gen3/Gen4 配置命令；
- 修改 manufacturer bytes 5..15；
- 将“写命令返回成功”视为 UID 已修改。

以上能力需要后续独立规格，不能复用普通 `Reader.WriteBlock` 绕过保护。

## 安全边界

普通 `Reader.WriteBlock` 继续拒绝 block 0 和 sector trailer。CUID/Gen2 写入通过独立、无 block 参数的 `ClassicManufacturerWriter` 能力进入；Gen1A 写入只能通过固定可执行文件名的外部适配器进入。GUI 和普通 restore 都无法把它们当成任意 block 写入后门。

任何 UID 写入必须按以下顺序执行：

1. 确认当前卡是 4-byte MIFARE Classic 1K，并验证 sector 0 密钥；
2. 读取 sector 0 trailer，确认访问条件允许使用已验证密钥写数据组 0；
3. 读取并验证当前 block 0 UID 与 BCC；
4. 生成只改变 UID/BCC 的新 block 0，并向用户展示；
5. 使用一次性、限时预检 token，要求用户再次输入完全相同的 UID；
6. 在独占设备锁内重新执行预检；
7. 持久化原始 block 0，备份失败则禁止写入；
8. 认证并发送 block 0 写命令，使用足够覆盖卡片 EEPROM 写周期的超时；无论最终响应是否确定，都主动复位 RF 场，使卡片重新载入防冲突 UID；
9. 重新寻卡；RF/超时/I/O 类最终响应只能表示提交状态未知，必须继续验证；出现意外 UID 或卡片消失直接失败；
10. 若明确重新检测到旧 UID，关闭进程内 reader，以参数数组将目标完整 block 0 交给 `nfc-mfsetuid`，完成后无论成功失败都重新打开 reader；
11. 使用新 UID 重新认证并读取 block 0，完整比对后才报告成功；外部工具退出码不能替代该验证。

## 自动化测试

- 4-byte UID BCC 自动生成；
- 7-byte/10-byte UID 和非法文本被拒绝；
- manufacturer bytes 5..15 保持不变；
- 普通 `Reader.WriteBlock` 继续拒绝 block 0；
- 专用 manufacturer capability 要求先认证，且不进行错误的同会话回读；
- 写命令成功但新 UID 未出现时判定验证失败；
- 普通写后旧 UID 未改变时只尝试一次 Gen1A fallback；移卡、换卡或目标出现意外 UID 时不得 fallback；
- Gen1A 工具使用选中设备的准确 connstring，参数为完整 16-byte block 0，且调用期间进程内 reader 已关闭；
- 原 block 0 备份先于写命令，备份失败时不写卡；
- 备份文件原子落盘且使用受限权限；
- 预检 token 一次性使用，目标 UID 必须精确二次确认；
- Dump 恢复后的访问条件不允许 block 0 写入时，在任何写入前拒绝组合任务；
- UID 变化后迁移卡片范围内的已验证密钥关联。

## 手工验收

- 在可报废 CUID/Gen2 测试卡上通过独立入口修改 UID；
- 在可报废 Gen1A 测试卡上确认普通写未改变 UID后自动切换，最终完成 UID 与完整 block 0 验证；
- 新 UID、BCC、manufacturer bytes 和备份文件全部验证；
- 加载带元数据的 Dump，确认恢复界面默认填入 Dump UID；
- 不勾选 UID 时恢复 Dump，确认目标卡 UID 不变；
- 勾选 UID 后恢复 Dump，确认数据、sector trailer 与 UID 均验证成功；
- 在普通不可写 UID 卡上尝试时安全失败，并显示原 block 0 与备份路径。

## 验收标准

- [x] 普通 restore 默认且始终通过普通路径跳过 block 0。
- [x] Dump 恢复和独立修改提供两个 UI 入口，并共用同一 UID workflow。
- [x] 每次 block 0 写入前持久化原 block 0。
- [x] 写入后必须重新寻卡并验证新 UID 与完整 block 0。
- [x] Gen1A 通过外部适配器和设备所有权交接实现；长 UID 和不可逆锁定功能未混入本期 UID 操作。
- [x] 失败报告包含原 block 0、目标 block 0、实际回读（如有）和备份路径（如已生成）。
- [ ] 完成当前 `E3 27 8A 8E` 卡的 Gen1A fallback 手工验收。
