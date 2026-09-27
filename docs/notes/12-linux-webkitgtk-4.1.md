# Linux WebKitGTK 4.1 运行时兼容

## 实际完成

- Linux Wails 开发、发布和全量 CGO 测试统一增加 `webkit2_41` build tag。
- GitHub Actions Linux 发布任务改为安装 `libwebkit2gtk-4.1-dev`。
- 保留 Ubuntu 22.04 构建基线，以避免无必要地提高 glibc 最低版本，同时兼容 Ubuntu 24.04 的 WebKitGTK 4.1。
- 发布任务新增 ELF `NEEDED` 校验，要求主程序链接 `libwebkit2gtk-4.1.so.0`，并拒绝遗留的 WebKitGTK 4.0 依赖。
- README 补充 Ubuntu 22.04+ 的 GTK/WebKitGTK 运行时安装命令，并说明 AppImage 不内嵌系统 WebKitGTK。

## 遇到的问题

Wails v2 在 Linux 未指定 WebKit build tag 时默认链接 WebKitGTK ABI 4.0。Ubuntu 24.04 提供 `libwebkit2gtk-4.1-0`，它不会提供 ABI 4.0 的 `libwebkit2gtk-4.0.so.37`，因此安装 4.1 无法满足原产物的动态链接依赖。

## 阻塞或未完成

需要由 GitHub Actions 重新生成 Linux AppImage/tar.gz，并在 Ubuntu 24.04 干净环境验证 GUI 启动。其余阻塞：无。
