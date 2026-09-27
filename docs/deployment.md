# NFCX 网站与遥测部署

本项目的桌面客户端会向 `https://telemetry.nfcx.tools/api/telemetry` 发起遥测请求。静态官网使用 `nfcx.tools`（Cloudflare Pages），遥测 API 与 Dashboard 使用独立的 `telemetry.nfcx.tools`（Cloudflare Worker）。

## 1. 部署网站到 Cloudflare Pages

静态官网已经位于 [`website/`](../website/)，可直接作为 Cloudflare Pages 项目的输出目录。若你选择自行用 Cloudflare Pages 托管，最简单的 Git 部署方式：

1. 将仓库推到 GitHub。
2. Cloudflare Dashboard → **Workers & Pages** → **Create application** → **Pages** → **Connect to Git**。
3. 选择此仓库；若网站是纯 HTML/CSS/JS，设置：**Build command** 留空，**Build output directory** 为 `website`。
4. 部署成功后，在 Pages 项目的 **Custom domains** 添加 `nfcx.tools` 和（可选）`www.nfcx.tools`。域名必须位于同一个 Cloudflare 账户的 zone 中。

先访问 Pages 给出的 `*.pages.dev` 地址确认首页正常，再配置正式域名。

## 2. 创建 D1 数据库

需要 Node.js 18+。在项目根目录执行：

```sh
cd telemetry-worker
npx wrangler login
npx wrangler d1 create nfcx-telemetry
```

最后一条命令会输出 database ID。将该值替换到 [`telemetry-worker/wrangler.toml`](../telemetry-worker/wrangler.toml) 的 `database_id`，不要改动 binding 名 `DB`。

将 schema 应用到远端 D1：

```sh
npx wrangler d1 execute nfcx-telemetry --remote --file=schema.sql
```

## 3. 部署 Worker 与 Dashboard

设置 Dashboard 密码；该命令会交互式读取密码，因此不会写入 Git：

```sh
npx wrangler secret put TELEMETRY_DASHBOARD_SECRET
npx wrangler deploy
```

本仓库的 `wrangler.toml` 已声明 `telemetry.nfcx.tools` 为 Worker Custom Domain。部署时 Cloudflare 会为这个子域名创建 DNS 与 TLS 证书；前提是 `nfcx.tools` 已在同一 Cloudflare 账户中作为 active zone。无需在 Pages 添加 `/api` 或 `/dashboard` 路由。

Dashboard 已写在 [`telemetry-worker/src/index.js`](../telemetry-worker/src/index.js)，同一个 Worker 会根据路径返回 API 或统计页面；因此它不需要、也不能作为静态文件上传到 Pages。

## 4. 验证

部署完成后，运行桌面程序、在首次弹窗保持“允许收集匿名信息”并点击继续。可以用：

```sh
curl -i https://telemetry.nfcx.tools/api/telemetry
```

确认得到 `404`（因为这里只接受 POST）；客户端发送的合法 POST 会得到 `204`。Dashboard 地址为：

```text
https://telemetry.nfcx.tools/dashboard?secret=你设置的密码
```

Cloudflare 的 D1 命令和 Workers route/Pages custom-domain 配置以其官方文档为准：[D1 Wrangler commands](https://developers.cloudflare.com/d1/wrangler-commands/)、[Workers routes](https://developers.cloudflare.com/workers/configuration/routing/routes/)、[Pages custom domains](https://developers.cloudflare.com/pages/configuration/custom-domains/)。
