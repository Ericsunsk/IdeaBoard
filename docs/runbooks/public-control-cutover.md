# Public / Control 平面切换手册（Issue #809 Final Hard Cut）

`apps/api` legacy `/api/user`、`/api/admin` 与单体 `openapi.yaml` 已删除；envelope 统一为 contract `{ data, meta? }` / `{ error: { code, message, request_id } }`。
`apps/web` 全部走 `generated-query/public/*`（prefix `/api/v1`）与 `generated-query/control/*`（prefix `/api/control/v1`）；
Better Auth 客户端（`AUTH_BASE_URL`）不受影响。Canvas 仍是产品主页，无路由重组。

> [!WARNING]
> 本次切换仅支持 fresh-DB 部署：legacy 鉴权字段（`auth_user_id`）、`storage_uid` 与旧 envelope 已无兼容读取。
> 已有数据盘不要直接升级；必须迁移请先做全量 PostgreSQL backup（密钥处理见 [data-protection.md](./data-protection.md)），
> 在 staging 用备份恢复演练后再动生产。

切换顺序（严格按序，前一步变绿再做下一步）：

1. Migrate（数据与代码迁移）
   - 后端先合入 River + PostgreSQL 事务提交（task 入队与状态同事务），确认 `apps/api/openapi.public.yaml` 与
     `apps/api/openapi.control.yaml` 为 contract 唯一来源。
   - 前端确认所有 admin 面经 resource/adapter 走 control client、account 面走 public client，
     page 层不直引 generated（`lint:import-boundaries` / `lint:dependencies` 变绿）。
2. Auth（鉴权面）
   - Better Auth 保持 `AUTH_BASE_URL` 同源；`sessionPolicy` 会话探针只认 `GET /api/v1/user`，
     admin 403-homeless 只认 `/control/` 前缀。换句话说：浏览器里再出现 `/api/user/info`、`/api/admin/*`
     请求一律视为 bug，直接按 P0 处理。
   - 按 [auth-operations.md](./auth-operations.md) 轮换 operator key / control key，确认 capability
    （`control:*` 通配与各 `control:xxx`）按最小权限下发。
3. API（后端上线）
   - 部署新镜像，确认 legacy 路由返回 404（`routes inventory test`），`/api/control/v1` 劫持
     `Idempotency-Key` + operator reason（缺失即 400），audit 记录完整。
4. Web / Proxy（前端与反代）
   - 发布 `apps/web/dist` 到 Cloudflare Pages；VPS 反代只保留 `/api/*`、`/api/auth/*`、`/uploads/*`
     三条转发（见 deployment.md 拓扑），删除一切 `/api/user`、`/api/admin` 兼容 location。
   - 前端发布后跑 `npm run api:generate && npm run api:check-generated`（zero drift），
     legacy grep 门（`/api/user|/api/admin` shipping 代码零命中、`message_safe` / `code===0` /
     `auth_user_id` / `storage_uid` 零命中）全绿。
5. Provider webhook URLs + signing verify（外部 provider）
   - Stripe：在 dashboard 把 webhook endpoint 换成新 control 回调地址，rotate signing secret，
     用 `stripe trigger` 在 staging 验证签名通过后再切生产流量；`StripeSettingsDialog` 仅做展示，
     不再读写 legacy stripe 配置面。
   - AI Gateway / Search provider / Storage：控制台回调与 key 指向新 control platform settings；
     brand logo 已改为前端直连 `https://models.dev/logos/{provider}.svg`（无后端透传），
     出网需放行 `models.dev:443`（Egress allowlist 即时生效）。
   - 所有 provider dashboard 改完后，在生产做一笔真实小额链路（下单→webhook→ledger）验证。

Provider dashboard 更新清单：Stripe webhook URL + signing secret、AI Gateway 回调、Search provider key、
Storage endpoint、Egress allowlist（`models.dev:443`）。

> [!CAUTION]
> Image rollback is not DB rollback：镜像回滚只能回退代码，PostgreSQL / NATS JetStream 中的
> River 任务状态、ledger、audit 记录不会跟着回去。新旧 envelope 不互认，
> 回滚旧镜像 + 新 DB = 全面 4xx/5xx。真要回滚：先停写流量 → 用升级前 backup 恢复 DB →
> 再切旧镜像 → 重放 River 队列校验。没有可用 backup 就不要回滚，只允许向前修复。

Breaking-change baseline freeze：本次切换冻结新 baseline = 当前 `openapi.public.yaml` + `openapi.control.yaml`
（web 端 `api:check-generated` zero drift 即冻结凭证）。后续任何 contract 改动必须先改 spec、
再 `api:generate`、再改调用方；`publicApiSurface.baseline.json`（`api:surface:check`）是第二道门。
