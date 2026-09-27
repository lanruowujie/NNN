# 17. macOS DMG 安装布局

## 实际完成内容

- macOS DMG 现在从临时 staging 目录创建，包含 `NFCX.app`、指向系统 `/Applications` 的 `Applications` 链接和安装提示文件。
- 用户打开镜像后可将 `NFCX.app` 拖到 `Applications` 目录完成安装。

## 实现过程中的问题

无。

## 阻塞或未完成事项

无。
