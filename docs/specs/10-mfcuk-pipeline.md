# 规格 10：MFCUK 到 MFOC 自动流程

## 目标

在没有已知密钥的授权测试卡上，提供一个可取消的两阶段流程：先由 `mfcuk` 尝试恢复第一把密钥，验证成功后自动交给 `mfoc` 恢复其余密钥。

## 前置条件

- 规格 09 已验收。
- 已固定可构建、可分发的 mfcuk 版本及其许可证。
- 已确认当前测试硬件支持所需的射频场控制和时序。

## 范围

- 实现 `MFCUKEngine`。
- 从受控输出格式或最小稳定文本中提取候选密钥、sector 和 key type。
- 通过 in-process libnfc 验证候选密钥。
- 验证成功后启动已有的 MFOC workflow。
- 在 GUI 中显示两个阶段及各自日志。
- 支持整个 pipeline 的取消、超时和失败恢复。
- 记录工具版本、参数、耗时和结果摘要。

## 不在本阶段实现

- 保证所有 Classic 卡都能恢复；
- 绕过 patched/fixed nonce 卡的防护；
- 自动选择 Hardnested；
- 对能力未知的读卡器默认启用该功能。

面向普通用户的主入口已由[规格 10A](10a-unified-key-recovery.md)统一编排；本规格的两阶段 pipeline 继续作为无种子密钥分支的内部能力和诊断接口保留。

## 流程

```text
preflight
  -> release device
  -> run mfcuk
  -> reopen device
  -> verify first key
  -> release device
  -> run mfoc with verified key
  -> reopen device
  -> verify recovered keys
  -> read dump
```

如果 mfcuk 输出多个候选，必须逐个验证，不能把第一个正则匹配结果直接视为成功。MFOC 只有在第一把密钥验证成功后才可启动。

建议为 NFCX 维护的 mfcuk 构建增加机器可读结果行，例如：

```text
NFCX_RESULT key=A sector=3 value=A0A1A2A3A4A5
```

这类最小补丁必须保留上游许可证和补丁记录。

## 自动化测试

- 零个、一个、多个候选结果；
- 候选密钥验证失败；
- mfcuk 成功但 mfoc 失败；
- 第一阶段和第二阶段分别取消；
- 两次设备释放/重开的顺序；
- 总超时与单阶段超时；
- 日志不会错误地显示未验证密钥为成功。

## 手工验收

- 使用专门的、确认适用的授权测试卡；
- GUI 在开始前显示能力和不保证成功的说明；
- mfcuk 找到候选后由 NFCX 验证；
- 验证成功才进入 mfoc；
- 完成后密钥表和 dump 可用；
- 任意阶段取消后设备恢复。

## 验收标准

- [ ] 两阶段状态在 GUI 中清楚可见。
- [ ] 未验证候选不会触发 mfoc。
- [ ] 每个阶段都有独立错误和日志。
- [ ] pipeline 可整体取消。
- [ ] 失败不会让读卡器永久处于 busy。
- [ ] 不适用的设备/卡片会被预检阻止或明确警告。
