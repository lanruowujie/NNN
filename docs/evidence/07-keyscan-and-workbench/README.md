# 规格 07 自动化验收证据：密钥扫描与数据工作台 GUI

日期：2026-09-12  
平台：macOS arm64  
Go：1.25.1  
Wails：2.15.0  
libnfc：固定版本 1.8.0

## 已通过检查

```sh
GOCACHE=/private/tmp/nfcx-go-cache go test ./...
GOCACHE=/private/tmp/nfcx-go-cache go vet ./...
GOCACHE=/private/tmp/nfcx-go-cache go test -race \
  ./app ./internal/keys ./internal/workbench ./internal/workflow
GOCACHE=/private/tmp/nfcx-go-cache make libnfc-binding-test
cd frontend && npm run build
GOCACHE=/private/tmp/nfcx-go-cache make build
```

`make build` 完成 Wails bindings 生成、前端编译、Go/libnfc 链接、`.app` 打包和本地自签名，产物为 `build/bin/NFCX.app`。

## 自动化覆盖

- 字典大小写和空白归一化、注释/空行、文件内和目录级重复、非法长度及非十六进制行号报告；
- 内置密钥来源、密钥库/扇区状态完整值、普通日志不记录密钥、用户密钥 `0600` 原子持久化和全部已知密钥显式导出；
- 同槽位成功密钥、同卡其他成功密钥、用户/导入密钥和内置字典的扫描优先级；
- Key A/Key B 独立认证、卡片身份复核、认证失败继续和取消后保留已验证结果；
- Hex/ASCII 16-byte round trip、dirty blocks、逐 byte diff、撤销、还原和重新加载；
- manufacturer/data/trailer 分类、BCC 和 trailer access-bit 校验及权限解释；
- 普通工作台、dump、编辑器和差异 DTO 的 trailer 完整密钥展示；
- 任务进度事件只报告扇区和 Key A/B 验证结果，不携带密钥值；
- 加载 dump metadata 后恢复当前卡片的已验证密钥；
- dirty-block restore 只计划选中地址，并继续跳过 block 0、数据块优先、trailer 最后和逐块验证；
- 写入预检失败时零写入、一次性指纹 token、卡片或工作副本变化后失效，以及 trailer 二次确认；
- 操作互斥、后台任务事件和真实上下文取消；
- 实时读取结果与已保存文件状态区分，以及任务完成后保存按钮恢复可用；
- 扫描、读取、保存/编辑的分步引导，以及无已验证密钥时禁止读取、部分/整卡读取状态区分；
- 快速任务完成事件与 Wails Promise 返回乱序时的终态保持，以及扫描/读取在硬件操作开始前同步清空旧工作副本；
- macOS 原生绿色缩放/全屏按钮的显式启用；
- 全部已知密钥字典导出、非零导出计数，跟随系统/浅色/深色主题切换，以及浅色主题统一配色变量；
- Vanilla TypeScript 前端类型检查和 Vite 生产打包；
- 默认与 `libnfc` build tag 两条 Go 构建路径。

所有测试使用确定性 mock reader 和合成 dump，不包含真实卡片数据或完整生产密钥。

## 手工验收状态

用户已在真实设备和测试卡上完成手工验收，包括密钥扫描与整卡读取、换卡后的工作台刷新、完整密钥显示、dump 保存、普通 block 写入及验证、全部密钥导出、窗口最大化/全屏和主题切换。验收期间发现的问题均已修正并复验。

用户于 2026-09-12 明确确认本次任务验收通过，规格 07 的验收清单已标记完成。
