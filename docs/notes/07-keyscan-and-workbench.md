# 规格 07 实施记录：密钥扫描与数据工作台 GUI

日期：2026-09-12

## 实际完成内容

- 新增 `internal/keys` 密钥领域层，提供 6-byte Classic 密钥解析、大小写/空白归一化、去重、稳定匿名 ID、来源记录和卡片/扇区/Key A-B 维度的验证状态。
- 内置 13 个保守的常见 transport/demo 密钥，来源固定到 libnfc 1.8.0 和 mfoc 0.10.7，并在 `internal/keys/data/SOURCES.md` 记录上游版本、commit 和路径。
- 用户可以在 GUI 中添加、删除、导入和导出密钥。导入保留合法行，并分别报告非法行号、文件内重复和与现有目录重复；用户目录原子保存到操作系统配置目录，目录权限为 `0700`、文件权限为 `0600`。
- 密钥库、扇区 Key A/Key B 状态、trailer block、ASCII 和差异预览均按产品要求显示完整值，不再生成星号、圆点或尾四位遮蔽形式。扫描任务日志只报告扇区与 Key A/B 的验证结果，不记录密钥值。
- 新增 `KeyScanService`，在一次 `DeviceManager.WithReader` 独占操作内对每个 sector 分别扫描 Key A/Key B。每次认证前重新确认卡片身份，认证失败作为普通候选失败处理，设备、卡片或上下文错误立即停止。
- 候选顺序固定为：当前卡片同一扇区/类型曾成功的密钥、当前卡片其他成功密钥、用户/导入/读取上下文密钥、内置字典。成功匹配先提交验证状态再发进度事件，因此取消不会丢失已确认结果。
- 新增 `internal/workbench` 权威内存模型，维护 baseline、working copy、逐 byte diff、dirty block、编辑历史、撤销、全部还原和保存后重置基线。
- Hex 编辑接受带空白的 32 位十六进制；ASCII 编辑接受最多 16 个 ASCII 字节，不足部分自动在尾部补 `00`，并拒绝超长或非 ASCII 输入。重新打开由该方式产生的零填充 block 时，编辑器会恢复零字节之前的文本。任一编辑只改变内存副本，并把该 block 标记为完整的 synthetic 内容。
- 工作台按 manufacturer/data/trailer 分类 block；表格、编辑器和 dirty diff 使用一致的完整 16-byte 内容。trailer 的危险提示、access bits 校验和写入二次确认保持不变。
- trailer 面板解码并解释四组 access condition，显示数据写权限、完整 trailer 写权限和 Key B 是否可读；非法互补位会阻止保存/写卡。
- GUI 已接通扫描、整卡读取、加载 `.bin`/`.mfd`/NFCX JSON 工程、保存、撤销、还原、差异预览、预检、写入、验证、进度和取消。
- `RestoreRequest.Blocks` 支持只写 dirty blocks；`nil` 仍保留规格 06 的整卡 restore 语义。两种模式都始终跳过 block 0，普通数据块仍先于 trailer 写入，并逐块验证。
- 写入采用五分钟有效的一次性预检 token，并绑定当前 dump 内容、卡片身份、选中 blocks 和源 UID 长度。工作台或卡片发生变化后旧 token 失效；包含 trailer 时还要求独立确认。
- 成功写入 trailer 后，使用新密钥完成的验证会更新当前卡片的 A/B 状态。加载带 metadata 的 dump 也会恢复其中已验证的卡片作用域密钥。
- 前端保持 Vanilla TypeScript/HTML/CSS，没有引入组件框架。桌面文件选择使用 Wails 原生对话框，取消选择不会改变当前工作台。
- 根据手工验收反馈，主界面移除了手动 connstring 输入，只允许选择 libnfc 自动枚举出的可用设备；窗口明确保持可调整大小，因此各平台保留最大化与系统全屏能力。
- 修复异步读取完成后保存按钮仍被任务态禁用的问题。工作台现在区分“未保存”“已保存”和“已修改”：实时读卡结果在成功写入文件前显示“未保存”，加载已有文件或成功保存后才显示“已保存”。
- 明确主操作顺序为“1 扫描已知密钥 → 2 读取卡片 → 3 保存或编辑”。没有任何已验证扇区时读取按钮不可用；部分扇区可认证时显示“读取可用扇区”，全部扇区可认证时才显示“读取整卡”。当前卡片已有验证记录时可以跳过重复扫描。
- 修复快速扫描时任务完成事件早于 Wails Promise 返回造成的前端竞态，避免任务被错误恢复为“运行中”并永久禁用读取按钮。扫描或读取任务被后端接受后，第一步均在启动硬件 goroutine 前同步清空旧工作副本并通知 GUI；读取结束后再载入当前卡片的完整或部分结果。仅换卡不会自动清空，以保留将已加载 dump 恢复到目标卡片的工作流；若工作副本有未保存修改，扫描和读取都会先要求确认。
- macOS 显式启用 Wails `Mac.DisableZoom=false`。Wails 在未提供 Mac options 时会把原生 `zoomable` 初值保留为关闭，仅设置通用 `DisableResize=false` 不足以启用绿色最大化/全屏按钮。
- 密钥字典导出由“仅导出非纯内置来源”调整为导出当前目录中的全部已知密钥，并在 GUI 日志中报告实际数量。这样只使用内置字典完成扫描时也不会再生成 0 字节文件，按钮和对话框文案同步改为“导出全部密钥”。
- 顶栏新增三态主题按钮，依次切换“跟随系统 / 浅色 / 深色”。默认不设置强制主题，由 `prefers-color-scheme` 跟随系统；显式选择写入浏览器本地存储，重启后保持。
- 根据手工反馈重新整理浅色主题：改用中性的系统灰白背景、白色面板、石墨色正文和低饱和青绿色主色，并统一卡片示意、表格、状态标签、进度条、滚动条及按钮的浅色变量，避免冷蓝灰背景与深色局部控件混用。

## 实现过程中遇到的问题

### trailer 明文展示与日志边界需要分开处理

规格 06 的 dump 可能包含通过认证上下文补全的完整 Key A/Key B。按本轮产品决定，常规工作台表格、编辑器和 diff 都直接展示这些字节；日志事件不携带密钥字段，也不输出任何遮蔽形式，从而避免用户误以为界面仍隐藏了密钥。

### dirty 写入不能复用“总是整卡”的 restore 计划

规格 06 的安全 restore 会预检并写入除 block 0 外的整卡。本阶段为其增加可选 block 集合，预检只认证和检查实际涉及的 sector，同时继续维持数据块优先、trailer 最后和逐块验证。空的修改集合不会退化成整卡写入。

### 预检结果在 UI 确认期间可能过期

用户查看差异和 access bits 后，卡片或工作副本可能已经变化。实现不直接让“确认”按钮重用旧参数，而是签发一次性 token；开始写入前重新比较卡片和工作副本指纹，任何变化都要求重新预检。

### macOS 原生文件对话框需要 UTI 框架

引入 Wails 文件选择后，macOS arm64 链接需要 `UniformTypeIdentifiers`。框架声明放在现有唯一允许 `import "C"` 的 `internal/nfc/libnfc` CGO 封装中。完整 Wails 构建还需要读取系统版本；受限沙箱会阻止该探测，允许系统只读探测后构建、打包和自签名均成功。

### 整卡读取耗时来自安全读取粒度

当前 Classic 1K 读取在开始时会复核每个扇区已提供的 Key A/Key B，随后对 64 个 block 逐块执行卡片身份确认、认证和读取。PN532 UART 上这些命令串行往返，因此数秒到十几秒属于预期范围，具体取决于串口链路、可用密钥数量和失败重试。实现没有为了缩短等待而移除换卡检测或逐块认证；若同一硬件稳定超过约 30 秒，应结合任务原始日志进一步定位设备超时或重复认证开销。

## 阻塞或未完成事项

- 自动化测试、竞态检查、静态检查、前端生产构建、固定 libnfc CGO 测试和 macOS arm64 Wails 完整构建均已通过。
- 用户已使用真实设备和测试卡完成扫描、读取、保存、换卡状态、写入、密钥导出、窗口控制与主题显示等手工验收，并于 2026-09-12 确认本次任务验收通过。
- Nested、Darkside、Hardnested、在线密钥库和特殊 UID 卡写入均按规格留在后续阶段。

本规格范围内无阻塞或未完成事项。

## 验收命令

```sh
GOCACHE=/tmp/nfcx-go-cache go test ./...
GOCACHE=/tmp/nfcx-go-cache go vet ./...
GOCACHE=/tmp/nfcx-go-cache go test -race \
  ./app ./internal/keys ./internal/workbench ./internal/workflow
GOCACHE=/tmp/nfcx-go-cache make libnfc-binding-test
cd frontend && npm run build
GOCACHE=/tmp/nfcx-go-cache make build
```

自动化结果和手工验收边界见 [`docs/evidence/07-keyscan-and-workbench/README.md`](../evidence/07-keyscan-and-workbench/README.md)。
