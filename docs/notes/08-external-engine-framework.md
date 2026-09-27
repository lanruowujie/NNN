# 规格 08 实施记录：外部引擎任务框架

日期：2026-09-13

## 实际完成内容

- 在 `internal/attack` 定义算法无关的 `AttackEngine`、`AttackRequest`、`AttackEvent`、`AttackResult` 和完整任务状态模型。请求不暴露可执行文件路径或任意参数数组，GUI 只能选择应用注册的逻辑引擎。
- 新增统一子进程运行器，使用 `exec.CommandContext` 和参数数组，不调用 shell。stdout 与 stderr 由独立 goroutine 使用固定大小 buffer 并发读取，因此支持超长无换行内容、任意分片和两路大量输出；结果保留两路原始字节，GUI 事件携带流类型和严格递增序号。
- 取消会终止整个进程树：macOS/Linux 为外部任务创建独立进程组，Windows 使用带 `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` 的 Job Object。用户取消与 deadline timeout 使用独立错误类型，非零退出保留退出码、版本和原始日志。
- 新增 `internal/runtime` 受控运行时定位器。逻辑引擎映射固定平台文件名，只能从应用或仓库的 `runtime/<os>-<arch>` 中解析；拒绝目录名、NUL、非普通文件、缺少执行权限和逃逸 runtime 根目录的符号链接，并支持发布构建提供预期 SHA-256。
- `DeviceManager` 新增外部设备交接事务：取得全局设备锁、停止并等待轮询、关闭当前 reader、执行外部回调、无条件使用独立 cleanup context 重开 reader并恢复轮询。执行错误与恢复错误分开返回，外部结果不会被恢复错误覆盖。
- 外部任务持有设备期间，`WithReader` 和设备重新枚举立即返回稳定的 busy 错误。GUI 同时禁用扫描、读写、刷新、连接切换等冲突操作。
- 精确 connstring 通过仅属于子进程的 `LIBNFC_DEVICE` 传递，同时设置该子进程的 `LIBNFC_AUTO_SCAN=false` 和 `LIBNFC_INTRUSIVE_SCAN=false`；没有修改系统 libnfc 配置或进程全局环境。
- 每次任务使用 `os.MkdirTemp` 创建独立目录，输出文件名由适配器生成。生产默认始终清理；测试和开发可以选择失败时保留，以检查诊断文件。
- 应用层复用现有全局异步任务槽、Wails `nfcx:task` 事件和取消按钮。任务事件增加 engine、version、stream、raw log、sequence、exit code 和独立 recovery error 字段；取消后的任务不会再发布 completed。
- 前端增加外部阶段显示和原始进程日志窗口。只在受控 runtime 中实际存在 fake engine 时显示“引擎框架自检”按钮，因此正式发布包不包含 fixture 时不会暴露测试入口。
- 新增 `cmd/nfcx-fake-engine`，可模拟成功、非零退出、挂起与子进程、缺失/损坏结果以及 stdout/stderr 双路大量长行输出。GUI 默认自检持续约三秒并逐步输出，让人工验收有时间点击取消；自动化测试使用零延迟。`make external-engine-fixture` 仅为本地开发/验收构建它，生成物被 Git 忽略且未进入 Wails 发布包。
- 根据产品确认更新安全边界：引擎使用随应用 bundle 的固定离线版本，不提供运行时自动下载；本阶段不加入操作系统级网络封锁，也不限制 NFCX 后续遥测功能。

## 实现过程中遇到的问题

### 设备独占不能复用 `WithReader`

`WithReader` 的契约是在 reader 保持打开时串行执行，而外部工具要求在整个子进程周期内关闭 reader、同时继续持有设备所有权。为此增加了独立的释放事务，而没有让 attack 层直接修改 `DeviceManager` 内部字段。设备重开使用与任务 context 分离的有界 context，避免取消或超时跳过清理。

### 取消完成与设备恢复存在竞态

子进程可能已经成功退出，但用户在结果校验或设备恢复阶段点击取消。最终状态现在以任务 context 为准，并在进程确认退出、设备恢复流程结束后才发布 terminal event。事件通过串行 sequencer 编号，保证 `cancelling` 不会落在最终 `cancelled` 之后。

### 默认 Scanner 不适合外部工具日志

`bufio.Scanner` 有默认 token 大小限制，并且等待换行会延迟无换行进度。实现直接读取 byte chunks，原始 stdout/stderr 分开保存；UI 只对无法显示的 UTF-8 字节做替换，不改变后端诊断字节。

### 网络安全边界需要与未来遥测兼容

经产品确认，“离线引擎”表示 NFCX bundle 并调用固定版本，不实现自动下载或网络工作流；不通过代理清理或三平台 OS sandbox 阻止进程联网。这样供应链入口仍受控，也不会给未来 NFCX 遥测增加需要删除的网络限制代码。

### 完整 Wails 构建需要系统只读探测

受限沙箱中的首次 `make build` 在 Wails 编译阶段被 macOS 系统探测阻止。允许该只读探测后，bindings、前端、CGO/libnfc 链接、应用打包和自签名全部完成。

## 阻塞或未完成事项

- 自动化测试、竞态检查、静态检查、前端生产构建、Windows/Linux attack 包交叉编译、固定 libnfc CGO 测试和 macOS arm64 Wails 完整构建均已通过。
- 2026-09-13 已通过连接真实读卡器的 GUI 手工验收：实时日志、取消、busy 状态和任务结束后的读卡器恢复均符合预期。验收时发现原始日志表单宽于所属 dialog，导致内容区和关闭按钮越界；已将宽度约束移至 dialog，并限制表单及日志内容不超过父容器。随后又发现密钥库空报告框会覆盖 `hidden` 且添加错误只写入弹窗后的任务日志；已恢复 `hidden` 语义，并让添加/导入结果在密钥库内以可访问状态提示完整显示。前端生产构建通过。
- Windows Job Object 已通过 Windows amd64 交叉编译，但需要在规格 12 的 Windows runner 上做真实进程树取消回归。
- 真正的 mfoc/mfcuk 参数、结果格式和密钥验证明确留在规格 09、10；本阶段无算法集成阻塞。

## 验收命令

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

自动化覆盖和手工验收边界见 [`docs/evidence/08-external-engine-framework/README.md`](../evidence/08-external-engine-framework/README.md)。
