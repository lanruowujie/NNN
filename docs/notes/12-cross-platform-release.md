# 规格 12：跨平台构建与发布实施记录

## 实际完成

- GitHub Actions 普通检查扩展为 macOS arm64、Windows amd64、Linux amd64 三个平台；
- 新增 tag/手工触发的跨平台发布工作流；
- Go 1.25.1、Wails 2.15.0、Node 24.8.0 和固定 runner OS 已写入工作流；
- 现有 libnfc、mfoc、mfcuk、mfoc-hardnested 构建脚本扩展到三个原生平台；
- Windows runtime 同时包含 `nfc-mfsetuid.exe` 和 hardnested 所需私有 liblzma DLL；
- 实现 macOS `.app`/DMG、Windows portable ZIP、Linux AppImage/tar.gz 打包；
- macOS 实现嵌套签名、主应用签名、公证和 stapling；
- Windows 在证书存在时使用 signtool 签名，否则产物明确带 `unsigned` 后缀；
- 新增 NFCX 构建信息注入、runtime manifest、文件 SHA-256 和整包 SHA256SUMS；
- 新增实际 Go runtime 依赖的许可证聚合；
- 新增无 GUI、无硬件的 `--self-check --json`；
- 新增 GUI“关于 / 诊断”页面，复用同一诊断服务；
- 发布校验包含架构、动态库依赖、外部工具能力、开发机绝对路径和许可证检查；
- 首版支持声明限制为 PN532 UART；Windows 首版使用 portable ZIP。

## 实现中遇到的问题

- Windows 的主程序在 Go 代码执行前就需要加载 CGO DLL，因此不能依赖运行后设置
  搜索路径。Windows portable 包采用应用根目录私有 runtime，DLL 和外部工具均从
  此目录解析。
- AppImage 1.9.1 默认下载可变的 continuous runtime。工作流额外固定 type2 runtime
  版本与 SHA-256，并通过 `--runtime-file` 传入，避免构建内容漂移。
- macOS manifest 必须在嵌套 Mach-O 签名之后生成，否则签名会改变文件摘要；主应用
  在 manifest 生成后签名，从而同时封装资源清单。
- 首次整包扫描发现 Go 可执行文件的调试元数据仍含开发机仓库绝对路径；Wails 构建已
  启用 `-trimpath`，重新打包后路径泄漏扫描通过。
- 首次 GitHub Actions 运行发现 Windows 不提供 POSIX 权限位语义；敏感文件仍以
  `0600` 请求创建，但权限位断言只在支持该语义的平台执行。
- Windows checkout 可能将 libnfc patch `series` 转为 CRLF，导致文件名末尾残留
  `\r`。仓库新增 LF 属性，并让解析器兼容已有 CRLF checkout。
- GitHub 源码下载偶发连接超时。下载器现在对连接类错误重试 6 次、使用临时文件，
  且失败时立即清理并停止，不再继续产生误导性的 checksum 错误。
- Linux 首次构建的 mfcuk 调试信息包含 runner 工作区路径。所有 C release 构建现以
  `-g0` 编译并 strip，且每个原生工具在构建阶段单独执行仓库路径泄漏检查。
- macOS 标签构建原先强制要求 Apple 签名与公证 secrets。现改为六个凭据全配或全不
  配：无付费开发者资格时仍生成带 `-unsigned` 后缀的 ad-hoc 签名 DMG，并明确不执行
  公证；部分配置会快速失败，避免生成签名状态不明确的产物。
- Windows MSYS2 的 `getconf ARG_MAX` 可能成功退出却输出 `undefined`，直接用于 Bash
  算术会在 `set -u` 下报未绑定变量。平台探测现只接受正整数，否则使用保守默认值；
  CPU 并行数也做了相同处理，并兼容 Windows 的 `NUMBER_OF_PROCESSORS`。
- libnfc 1.8.0 的 Autotools 路径无法识别现代 MinGW host triplet，因而在 Windows 错误
  编译 POSIX UART/日志实现并缺少 `err.h`。Windows 现改用上游已有的 CMake Windows
  实现，显式只启用 `pn532_uart`，并补齐旧安装规则遗漏的 MinGW import library 与
  `libnfc.pc`；macOS 和 Linux 仍使用原有 Autotools 构建。
- libnfc 1.8.0 自带的 Windows `setenv`/`unsetenv` 兼容代码把字符缓冲区误声明为指针
  数组，现代 GCC 会拒绝编译且原固定长度实现存在溢出风险。仓库补丁改用 MinGW-w64
  的 `_putenv_s`，并清理新版头文件中 errno 常量重复定义的警告。
- libnfc 1.8.0 的 CMake 在 Windows 主机上无条件追加 `-m32`，导致 MINGW64 linker 将
  `wsock32`、`kernel32` 等所有 64 位系统库判为不兼容。仓库补丁现按 CMake 检测到的
  指针宽度选择资源格式，并保留编译器自身的 x86_64 target。
- Windows CMake 的全部示例中有两个诊断程序调用 DLL 未导出的内部 PN53x 符号，虽与
  NFCX 无关仍会阻断全量构建。Windows 现只构建 `nfc-mfsetuid` 目标及其依赖，并暂存
  NFCX、CGO 和外部引擎实际需要的 DLL、import library、头文件及 pkg-config 元数据。
- Windows 的 MinGW-w64 不提供 BSD `err.h`，而 `mfoc`、`mfcuk` 和 `mfoc-hardnested`
  都复用了包含该头的 libnfc 示例辅助代码。Windows libnfc SDK 现从同一份锁定源码
  生成自包含的上游兼容头，并在 SDK 校验阶段强制检查；该副本也补齐了自身使用的 `stdio.h`，
  避免三个外部引擎逐个遇到相同的编译失败。
- mfoc 0.10.7 把 `errno` 用作 `usage` 函数的形参名，现代 MinGW-w64 会将它展开为
  runtime accessor 宏，导致函数签名被错误解析。锁定源码现应用最小补丁，将形参重命名为
  `exit_code`；补丁和 source lock 一并进入 runtime，供发布审计和对应源码复现。
- mfcuk 0.3.8 的 configure 错误地强制要求 Linux、BSD 或 macOS 的 endian 头文件之一，
  尽管实际源码已为 GCC 使用 `__builtin_bswap*` 并提供通用回退。NFCX 的锁定集成补丁现
  保留可选头探测但移除该过时门槛，使 MinGW-w64 使用既有编译器实现。
- mfcuk 的 Windows 条件分支检查旧式 `WIN32`，而当前 MinGW-w64 提供标准 `_WIN32`，
  导致构建误入依赖 `select()` 的 POSIX sleep 实现。集成补丁现统一检查 `_WIN32`，复用
  上游已有的 `Sleep()`、`xgetopt` 和 Windows 清屏实现。
- Windows release job 的 MSYS2 shell 默认只继承最小系统 PATH，会过滤 `setup-go` 和
  `go install` 加入的 Go/Wails 路径；原生 C 工具链因此能构建，但随后 Makefile 无法执行
  `go env` 并误报 libnfc SDK 缺失。MSYS2 现显式继承 runner PATH，使 binding test 和
  Wails 应用构建使用已锁定安装的 Go 1.25.1 与 Wails 2.15.0。
- Windows CGO 测试进程由系统 loader 解析 `libnfc.dll`，不会读取 Unix 的
  `LD_LIBRARY_PATH`/`DYLD_LIBRARY_PATH`。binding test 现同时将平台 runtime 前置到
  `PATH`；Wails build/dev 生成绑定时也会执行带 CGO 的临时程序，现复用同一 runtime
  PATH。路径扫描测试则改用 `t.TempDir()` 派生的本机绝对路径，不再硬编码 Unix 路径。
- 三个平台的 release job 新增锁定原生 runtime 缓存，覆盖校验过的源码 archive、开发
  SDK 和 runtime；key 只绑定真正决定原生产物的工具链脚本、来源锁和所有补丁，不因
  应用层 Makefile 包装调整而失效。缓存命中仍执行
  libnfc smoke 与四个工具的 verify，缓存缺失则在构建成功后立即保存，后续打包失败不会
  浪费已完成的原生构建。
- Linux 自检发现短生命周期进程的输出末尾可能在 `Wait` 关闭 pipe 时丢失，恰好截掉
  `mfoc-hardnested` 帮助末行的版本。进程收集器现让 `os/exec` 在 `Wait` 返回前完整排空
  stdout/stderr，同时继续实时转发两个流。
- Linux Wails 产物原先在 Ubuntu 22.04 上按默认标签链接 WebKitGTK 4.0，导致仅提供
  WebKitGTK 4.1 的 Ubuntu 24.04 无法加载应用。发布构建继续使用 Ubuntu 22.04 保持较老
  glibc 基线，但改装 4.1 开发包并统一加入 `webkit2_41` 构建标签；工作流会检查最终
  ELF 明确依赖 `libwebkit2gtk-4.1.so.0` 且不再依赖 4.0。

## 阻塞或未完成

- 本地 macOS 环境已从锁定源码全量重建原生依赖，并完成签名顺序、manifest、29 个
  runtime 文件摘要、命令行自检、开发机路径扫描、未签名 DMG 生成及镜像校验；修复
  后又在 Ubuntu 22.04 amd64 容器完成 libnfc、mfoc、mfcuk、hardnested 全量原生构建、
  smoke test 和路径扫描。GitHub-hosted Windows 以及 Linux 完整 Wails 打包仍须由下一次
  Actions 运行验证。
- Developer ID、Apple 公证和 Windows 代码签名依赖 repository secrets，仓库内不能
  自行完成真实签名验证；不提供 Apple 凭据时会发布明确标记的 unsigned DMG。
- 三个平台干净机器和 PN532 UART 手工验收尚未执行，记录位置已建立。
