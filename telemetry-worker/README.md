# NFCX telemetry Worker

Dashboard 和 API 都在 [`src/index.js`](src/index.js) 中，由 Cloudflare Worker 提供；它们不属于 Cloudflare Pages。Worker 使用专用子域名 `telemetry.nfcx.tools`：

- `POST https://telemetry.nfcx.tools/api/telemetry` — 桌面客户端匿名事件接口；
- `GET https://telemetry.nfcx.tools/dashboard?secret=...` — 管理 Dashboard。

从本目录执行：

```sh
npx wrangler login
npx wrangler d1 create nfcx-telemetry
# 将上一步输出的 database_id 填入 wrangler.toml
npx wrangler d1 execute nfcx-telemetry --remote --file=schema.sql
npx wrangler secret put TELEMETRY_DASHBOARD_SECRET
npx wrangler deploy
```

`wrangler.toml` 已将 `telemetry.nfcx.tools` 配为 Worker Custom Domain。首次部署前，`nfcx.tools` 必须已在同一 Cloudflare 账户中成为 active zone；Cloudflare 会为该子域名创建 DNS 与证书。

The Worker only writes the allowlisted installation ID, event, app version, OS family and its own database timestamp. It never stores request bodies, headers, IP addresses, NFC data, or arbitrary properties.
