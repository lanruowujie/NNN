# 规格 08 自动化验收证据：外部引擎任务框架

日期：2026-09-13  
平台：macOS arm64  
Go：1.25.1  
Wails：2.15.0  
libnfc：固定版本 1.8.0

## 已通过检查

```sh
GOCACHE=/tmp/nfcx-go-cache go test ./...
GOCACHE=/tmp/nfcx-go-cache go vet ./...
GOCACHE=/tmp/nfcx-go-cache go test -race \
  ./app ./internal/attack ./internal/device ./internal/runtime
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 GOCACHE=/tmp/nfcx-go-cache \
  go test -c -o /tmp/nfcx-attack-windows.test ./internal/attack
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOCACHE=/tmp/nfcx-go-cache \
  go test -c -o /tmp/nfcx-attack-linux.test ./internal/attack
GOCACHE=/tmp/nfcx-go-cache make libnfc-binding-test
cd frontend && npm run build
GOCACHE=/tmp/nfcx-go-cache make build
```

`make build` 完成 Wails bindings、Vanilla TypeScript/Vite、Go/CGO/libnfc 链接、`.app` 打包和本地自签名。检查发布产物确认其中没有 `nfcx-fake-engine`、`mfoc` 或 `mfcuk`。

## 自动化覆盖

- 受控 runtime 查找、固定平台文件名、执行权限、SHA-256、路径穿越和符号链接逃逸；
- 参数和临时目录包含空格与非 ASCII 字符时保持原值；
- 精确 `LIBNFC_DEVICE` 子进程环境传递，参数不经过 shell；
- stdout/stderr 并发大量、无换行长输出的实时分片与完整捕获；
- 事件序号及 queued、release、start、run、validate、reopen、terminal 顺序；
- fake engine 成功结果、非零退出码 17、缺失/损坏结构化结果；
- 用户取消与 deadline timeout 的独立错误和终态；
- Unix 进程组中的 fake child 在取消时被真实终止；
- Windows Job Object 和 Linux process-group 源码的目标平台交叉编译；
- 成功、引擎失败、取消时都调用设备重开；
- 重开失败与原始引擎结果分开保存；
- 外部设备占用期间普通 Reader 操作和设备枚举返回 busy；
- 默认临时目录清理以及失败诊断保留策略；
- Wails DTO 的原始流、退出码、版本和恢复错误序列化；
- 取消后的应用任务不会发布伪 completed 事件；
- TypeScript 类型检查和前端生产构建。

所有进程测试使用仓库内的 fake engine，不包含真实破解工具、真实卡片数据或用户密钥。

## GUI 冒烟检查

构建本地 fixture 后通过 `make dev` 启动 NFCX，确认“引擎框架自检”按钮仅在 fixture 可解析时出现，原始日志入口存在，未连接读卡器时自检按钮保持禁用。本次环境没有枚举到读卡器，因此没有执行设备释放/重开的手工硬件步骤。

## 手工验收状态

2026-09-13 已在开发模式连接真实读卡器完成 GUI 验收：fake engine 可启动并实时输出日志，任务期间普通 NFC 操作为 busy，取消会终止任务，成功和取消后读卡器均可恢复并继续寻卡。

验收截图暴露出原始日志表单的 920px 宽度超过通用 dialog 的 780px 宽度，内容区及关闭按钮因此越界。现已把日志专用宽度设置到 dialog 本身，并为内部表单和日志区域补充父容器宽度约束；`npm run build` 验证通过。

后续界面检查还发现密钥库的空导入报告框错误显示，且非法密钥的具体错误只记录在弹窗后的任务日志。现已明确保证带 `hidden` 的元素不参与布局，并把添加密钥和导入字典的成功或失败结果直接显示在密钥库弹窗内；错误提示使用 alert 语义。前端生产构建再次通过。
