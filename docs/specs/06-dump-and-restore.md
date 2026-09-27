# 规格 06：Dump、Restore 与安全校验

## 目标

实现 MIFARE Classic 整卡备份、文件加载和受保护恢复，并将现有 Python 原型中验证过的 BCC、access bits、trailer 顺序和写后校验规则正式化。

## 前置条件

- 规格 05 已验收。

## 范围

- 使用已知的逐扇区 Key A/Key B 读取整卡。
- 保存兼容的 raw `.bin`/`.mfd` 文件。
- 保存并列的 NFCX JSON 元数据。
- 加载并验证 1024/4096 字节 dump。
- 校验 4-byte UID 卡的 block 0 BCC。
- 解码并校验 sector trailer access bits。
- 提供预检、执行、验证三个明确阶段。
- 普通数据块优先、sector trailer 最后写入。
- 对每个写入 block 进行回读验证。
- 生成可供 GUI 展示的逐块结果。

## 不在本阶段实现

- block 0 写入；
- 未知密钥恢复；
- 自动修复无效 access bits；
- 把 partial dump 冒充完整 dump。

## Dump 模型

每个 block 必须有独立状态：

```text
unread
read
auth_failed
read_failed
synthetic
verified
```

Raw dump 只能在所有要求的 block 都有确定内容时标为“完整”。部分读取可以保存为 NFCX 工程/元数据，但必须显著标记缺失块，不能无提示地用零填充后作为完整卡片写回。

sector trailer 中因硬件/协议无法直接读出的 Key A，应从实际成功使用的密钥补入 dump，并在元数据中记录其来源。不得猜测未知密钥。

## Restore 安全流程

1. 验证文件长度和结构；
2. 识别目标卡并确认容量匹配；
3. 确认任务开始和写入过程中 UID 不变；
4. 在写入前认证所有目标 sector；
5. 校验每个 trailer 的 access bits；
6. 先写普通数据块；
7. 每块写后回读；
8. 最后写 trailer，并用新密钥重新认证；
9. 失败时停止并生成已写 block 清单；
10. UI提供勾选，决定是否也写入 block0 uid；默认跳过 block 0。

## 自动化测试

- 1K/4K dump 长度验证；
- BCC 正确、错误和不适用场景；
- access bits 的有效/无效组合；
- trailer 写入排序；
- Key A 补全来源记录；
- partial dump 不可直接 restore；
- 中途失败时结果能准确列出已写/未写 block；
- metadata round trip。

测试 fixture 不得包含真实生产卡的敏感数据。

## 手工验收

- 从测试卡生成 1024 字节 dump；
- 关闭应用后重新加载，内容和元数据一致；
- 恢复到授权测试卡，跳过 block 0；
- 所有已写 block 回读一致；
- 损坏一个 trailer access bits 后，预检阶段拒绝执行且不写任何块；
- 写入过程中移卡时立即停止并显示已完成范围。

## 验收标准

- [x] raw dump 与常见 `.bin`/`.mfd` 工具兼容。
- [x] 无效文件在写卡之前被拒绝。
- [x] restore 默认不写 block 0。
- [x] trailer 最后写，并验证新密钥。
- [x] 每个写入 block 都有回读结果。
- [x] partial dump 不会被误标为完整。
- [x] 失败报告足以判断卡片可能处于什么状态。
