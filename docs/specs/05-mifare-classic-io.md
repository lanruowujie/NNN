# 规格 05：MIFARE Classic 基础读写

## 目标

通过 in-process libnfc 完成 MIFARE Classic 已知密钥认证、读块和受控写块，为整卡 dump/restore 奠定基础。

## 前置条件

- 规格 04 已验收。
- 有一张自有测试卡，且至少知道一个测试扇区的密钥。

## 范围

- 定义 Key A、Key B 和严格的 6 字节 Key 类型。
- 定义 Classic 1K 和 4K 的 sector/block 几何结构。
- 实现指定 block 的 Key A/Key B 认证。
- 实现读取 16 字节 block。
- 实现写入 16 字节 block。
- 将 MIFARE 命令字节集中在底层定义。
- 处理认证状态在重新选卡或换卡后的失效。
- 对单块写入执行写后回读。

## 不在本阶段实现

- 整卡 dump/restore；
- 自动破解未知密钥；
- block 0 写入；
- sector trailer 编辑 GUI；
- value block 增减操作。

## 实现要求

支持的命令集中定义：

```text
0x60 Authenticate Key A
0x61 Authenticate Key B
0x30 Read
0xA0 Write
```

业务层不得出现未命名的上述魔法数字。认证必须关联当前卡片身份；任务中途 UID 变化时立即失效并中止。

Classic 几何函数至少提供：

- block 属于哪个 sector；
- sector 的首 block；
- sector trailer block；
- sector 的 block 数量；
- 卡型的总 block 数和 dump 大小。

写入 API 默认拒绝 block 0。sector trailer 可以在底层写入，但上层必须显式标记这是 trailer，后续规格再开放安全流程。

## 自动化测试

- Classic 1K 的 16 个 sector/64 个 block 映射；
- Classic 4K 的 40 个 sector/256 个 block 映射；
- 非法 key 长度和 block 编号被拒绝；
- Key A/Key B 映射正确；
- 写后内容不一致返回独立错误；
- 卡片 UID 变化导致正在执行的操作中止。

## 手工验收

- 使用默认 Key A 认证测试卡 sector 0；
- 读取 block 0 和一个普通数据 block；
- 对专用测试卡写入一个非 trailer 数据 block；
- 回读结果与输入完全一致；
- 使用错误密钥得到“认证失败”而不是笼统超时；
- 尝试普通 API 写 block 0 被拒绝。

## 验收标准

- [x] Key A 和 Key B 都有清晰类型和测试。
- [x] 1K/4K 几何计算测试通过。
- [x] 已知密钥认证和读块成功。
- [x] 非 block 0 数据块可安全写入并验证。
- [x] 错误密钥、移卡、换卡和设备断开均有可判定错误。
- [x] 普通写 API 默认保护 block 0。
