# NFCX 实施规格索引

本目录把 NFCX 的开发拆成可以顺序实施、单独验收的规格。项目级架构和长期约束以仓库根目录的 `AGENTS.md` 为准；若单个规格与其冲突，应先更新架构决定，再修改规格，不要在实现中静默偏离。

## 使用方式

1. 严格按编号实施，除非规格明确允许并行。
2. 开始某一阶段前，确认其“前置条件”全部满足。
3. 只实现“范围内”功能，不顺手加入后续阶段功能。
4. 完成本阶段的自动化测试和手工验收。
5. 将验收证据记录在 PR、提交说明或对应 issue 中，再进入下一阶段。

## 规格列表

| 顺序 | 规格 | 交付结果 |
|---:|---|---|
| 01 | [项目骨架与 GUI 外壳](specs/01-project-scaffold.md) | Go 1.25.1 + Wails v2 应用可启动 |
| 02 | [固定 libnfc 工具链](specs/02-libnfc-toolchain.md) | 可复现的 libnfc 和平台运行时布局 |
| 03 | [libnfc C shim 与 Go Reader](specs/03-libnfc-binding.md) | 主进程可通过稳定 Go 接口调用 libnfc |
| 04 | [设备管理与寻卡](specs/04-device-and-card-discovery.md) | GUI 可连接读卡器并显示卡片信息 |
| 05 | [MIFARE Classic 基础读写](specs/05-mifare-classic-io.md) | 已知密钥认证、读块、写块可用 |
| 06 | [Dump、Restore 与安全校验](specs/06-dump-and-restore.md) | 整卡备份与受保护恢复可用 |
| 07 | [密钥扫描与数据工作台 GUI](specs/07-keyscan-and-workbench.md) | 密钥管理、扇区表格、编辑体验完整 |
| 08 | [外部引擎任务框架](specs/08-external-engine-framework.md) | 可安全运行、取消并监控外部工具 |
| 09 | [MFOC Nested 集成](specs/09-mfoc-integration.md) | 已知一把密钥时可恢复其余密钥 |
| 10 | [MFCUK 到 MFOC 自动流程](specs/10-mfcuk-pipeline.md) | 无已知密钥时可尝试恢复第一把并继续 |
| 10A | [统一密钥恢复入口](specs/10a-unified-key-recovery.md) | 常见密钥、Darkside、Nested 与读取验证由一个任务编排 |
| 10B | [MFOC-Hardnested 集成](specs/10b-hardnested-integration.md) | Nested 未完整恢复时可执行 Hardnested |
| 11 | [特殊 UID 卡操作](specs/11-special-uid-cards.md) | CUID/Gen1A 等操作被明确区分并保护 |
| 12 | [跨平台构建与发布](specs/12-cross-platform-release.md) | Windows、macOS、Linux 可安装产物 |
| 16 | [低打扰的应用内更新](specs/16-in-app-update.md) | 静默检查、受控下载与安全重启更新 |

## 每阶段通用完成定义

除各规格的专用验收条件外，每一阶段都必须满足：

- `go test ./...` 通过；
- 新代码经过格式化，且没有未解释的静态检查错误；
- 普通自动化测试不要求连接真实读卡器；
- 不把本机串口路径、用户名、绝对路径或真实卡片密钥写入源码；
- GUI 中的长任务不会阻塞窗口；
- 新增第三方代码或二进制时记录来源、commit 和许可证；
- 文档与实际行为一致。

## 版本范围

首个可发布版本的主要目标是 MIFARE Classic 1K。Classic 4K 的几何结构和 dump 校验应在领域层预留并测试，但不要求所有 GUI 工作流在第一版对 4K 宣称正式支持。
