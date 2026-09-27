# 规格 10 验收证据

## 自动化与构建

- MFCUK 0.3.8 源码归档 SHA-256 校验通过。
- NFCX 机器结果补丁可重复应用，MFCUK 可链接固定 libnfc 1.8.0 构建。
- runtime 的版本、机器结果标记、动态依赖和 `@executable_path` rpath 校验通过。
- Go 单元测试覆盖候选零个/一个/多个、去重、认证拒绝、禁止无验证候选启动 MFOC、第二阶段失败保留验证种子、第一阶段取消恢复设备及两次设备交接顺序。
- Wails binding、TypeScript 和 Vite 生产前端构建通过。
- macOS arm64 应用 bundle 包含并签名 libnfc、MFCUK、MFOC、许可证、源码锁和 MFCUK 补丁。

## 手工验收步骤

1. 完全退出任何旧 NFCX 实例，启动 `build/bin/NFCX.app`。
2. 点击“刷新设备”，连接 PN532 + FT232RL 对应的 `pn532_uart` 设备。
3. 放入一张自有或明确获准测试、没有已录入验证密钥的 MIFARE Classic 1K 测试卡。
4. 确认设备详情同时显示 `Nested` 与 `Darkside`，底部 `MFCUK → MFOC` 按钮可用。
5. 点击按钮，阅读授权与兼容性提示，再点击“确认授权并开始”。
6. 观察阶段 1/2 MFCUK、设备恢复、候选复验、阶段 2/2 MFOC、再次恢复、密钥复验和读卡状态。MFCUK 静默恢复期间进度条应持续往返，每 5 秒显示进程心跳；产生实际认证尝试后还应显示 Sector、Key 类型和尝试次数。
7. 打开“原始日志”，确认 stdout/stderr 带有 `mfcuk` 或 `mfoc` 引擎标识；普通状态不得把尚未验证的候选称为成功密钥。
8. 完成后确认扇区密钥表可用、工作台出现重新读取的 dump，并可正常继续保存。
9. 另行运行一次，在 MFCUK 阶段取消；确认进程结束、设备恢复轮询且不会进入 MFOC。
10. 如条件允许，再在 MFOC 阶段取消；确认第一阶段已经由 libnfc 验证的密钥仍保留，设备恢复可用。

## 预期限制

- 入口只支持 Classic 1K；4K 卡不会启用。
- 卡片已有任何 NFCX 已验证密钥时，应直接使用 `MFOC Nested`，MFCUK pipeline 不启用。
- fixed nonce、patched nonce 或时序不适用的卡可能无法恢复，这属于明确支持的失败结果。

## 手工验收结果

待用户使用授权测试卡填写。
