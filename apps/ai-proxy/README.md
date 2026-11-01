# AI Proxy（Cloudflare Worker）

`apps/ai-proxy` 是唯一的 AI 出口适配器（Cloudflare AI Gateway sole egress 的边缘侧）。
VPS 后端只发 CanonicalImageRequest，图片输入/输出全走 presigned URL，Worker 持有网关密钥。

## 冻结合同（唯一出口）

- `POST /v1/generate`，`Authorization: Bearer WORKER_INTERNAL_SECRET`。
  Body `CanonicalImageRequest`：`task_id`、`provider: workers-ai|fal|openai`、`model`、`prompt`、
  `negative_prompt?`、`width?`、`height?`、`steps?`、`guidance_scale?`、`seed?`、
  `images?: string[]`（presigned GET URL）与可选 `mask_index?: number`（显式零基 mask 索引）、`provider_options?`、
  `upload_url`（必填）、`public_url?`。
- 成功：`{task_id,status:completed,provider,model,image_url,content_type,seed?,duration_ms,receipt:{upstream_status,provider,model,log_id?,cache_status?}}`。
- 失败：`{task_id,status:failed,error:{code,message,upstream_status?}}`。
  queue 型（202 / `request_id` + queue status）fail-closed 报 `UNSUPPORTED_MODEL`（422），绝不轮询。
- `POST /v1/dry-run`（`upload_url` 可选）：回 `{provider,model,target:{url,method,body},path_source}`，
  headers 永不返回，body 内 `token/key/secret/auth` 全打码为 `***`，image bytes 占位。
- presigned GET 拉输入：上限 10MB + `content-type: image/*` 检查，失败 `INPUT_FETCH_FAILED`（400）。
- presigned PUT 直传输出：exact key，原样 PUT，不改写 URL；失败 `UPLOAD_FAILED`（502）。Go 输出槽的 HMAC 绑定 `worker-image-v1` 固定策略（仅 PNG/JPEG/WebP），同时绑定 method/key/expiry；上传 header 必须与真实图片字节一致，API 校验 image config、大小和尺寸后按真实 MIME 存储，ingestion 再核对 Worker 回包、存储 MIME 与图片字节。槽 key 的 `.png` 后缀仅是 opaque Object Key，不决定 MIME。
- Go Worker 输出 URL 始终经 API HMAC route；Local/S3 后端共用同一验证后再写入对象，S3 读取走独立 presigned GET 并使用对象真实 MIME。未放宽浏览器 upload 的 S3 签名合同。
- 所有 Gateway transport（Fal/OpenAI 与携带 gateway id 的 `@cf/` Airun）强制 `cf-aig-skip-cache: true`、`cf-aig-collect-log-payload: false`，`provider_options` 无权覆盖。
- S3/网关密钥永不进 Worker 日志；`safeLog` 只记 `task_id/provider/model/path_source/upstream_status/duration_ms/code`。
- `WORKER_INTERNAL_SECRET` 缺失即 fail-closed：任何请求都 401。

## Endpoint slug（置顶常量，见 `src/providers/airun/index.ts` 与 `src/providers/gateway/index.ts`）

`prepareRoute` 在一次请求中只解析一次 channel/vendor，并把 target 与对应回包 decoder 绑定；generate 与 dry-run 共用该入口。`path_source` 仅作诊断，OpenAI endpoint 由实际 JSON/multipart input 形态选择。纯类型、错误与通用策略位于 `providers/shared.ts`，channel/vendor 不反向依赖分发根。

channel 决定 transport（host/auth/envelope/回包），vendor 只做 input 字段映射：

- airun（`POST https://api.cloudflare.com/client/v4/accounts/{account}/ai/run`，
  body `{model, input}`，`Authorization: Bearer CF_AIG_TOKEN`，`@cf/` 模型另带
  `cf-aig-gateway-id`）：`google/*`（nano-banana 系）、`openai/gpt-image-*`、
  `krea/*`、`workers-ai` 旧 slug（migrated，dry-run 标注）。判定规则见
  `src/providers/index.ts resolveChannel`：池条目 provider 声明优先，其次模型前缀回落。
- gateway（`https://gateway.ai.cloudflare.com/v1/{account}/{gateway}` +
  `cf-aig-authorization` + provider 原生 path）：`fal/{model}`（仅同步；绝不设
  `x-fal-target-url` 指向 `queue.*`）、`openai/images/generations`（无图 JSON）/
  `openai/images/edits`（有图 multipart）。

GET Probe（如 `/v1/models`）验不出计费/模型可用性与 payload 方言，必须首单真生成确认。

## Image 映射（首单待确认见下）

图片角色合同：`images` 保留普通参考图顺序（最多 4 张普通图）；可选 `mask_index` 是其中 mask 的零基索引，mask 另占一张，且必须至少有一张普通输入图。不传表示全部为普通参考图。Fal 保留全部参考图，仅显式 mask 映射为 `mask_url`；Gateway OpenAI edits 与 migrated Workers AI 只支持一张普通输入加可选 mask，多参考图明确拒绝。Airun Google/OpenAI/Krea 保留普通参考图，但目前拒绝显式 mask。dry-run 使用相同校验和角色映射。 `provider_options` 不允许注入图片或 mask 角色字段。

- airun/google（nano-banana 系）：无图 text-to-image；有图 `image_input`（data URI，≤3 张）；
  `aspect_ratio` 缺省有图 `match_input_image`/无图 `1:1`，`output_format` 缺省 `png`，
  `resolution` 缺省 `1K`；`provider_options` 仅允许这三 key（超集直接 422）。
- airun/openai（gpt-image 系）：`images`（data URI）+ `quality/size/background/output_format`
  Merge 透传；`size` 缺省由 canonical width/height 合成。
- airun/krea：`prompt + provider_options` Merge，有图 `image_urls`（data URI，P0 待确认）。
- workers-ai 迁移：单张普通图转 `image_b64`，可选显式 mask 转 `mask_b64`；多张普通输入明确拒绝。
- gateway/fal：无图 `{prompt,image_size,num_inference_steps,guidance_scale,seed}`；
  有图转 data URI（不透传 presigned URL）：`image_url` = 首张，`image_urls` = 全部普通参考图，`mask_url` 仅来自显式 `mask_index`。
- gateway/openai：无图 `images/generations {model,prompt,size,response_format:b64_json}`；
  有图 `images/edits` multipart（只支持一张普通 `image` 加可选显式 `mask`，`size/response_format` 同带）。

## 环境变量

| 变量                     | 说明                                                                   |
| ------------------------ | ---------------------------------------------------------------------- |
| `CF_ACCOUNT_ID`          | Gateway account id（线上 vars；本地从 Infisical dev 写入 `.dev.vars`） |
| `CF_GATEWAY_ID`          | Gateway id（线上 vars；本地从 Infisical dev 写入 `.dev.vars`）         |
| `CF_AIG_TOKEN`           | Gateway token（线上 Secret；本地仅 `.dev.vars`）                       |
| `WORKER_INTERNAL_SECRET` | 后端→Worker 鉴权（线上 Secret；本地仅 `.dev.vars`；缺失即 401）        |

## 运行与验证

```bash
cd apps/ai-proxy
npm install
npm run build   # tsc --noEmit
npm test        # providers 单测（录制 fixture，不外发）+ handler/dry-run 自测
npm run test:contract # Go 编码 → 真实 Edge 入口 → Go 解码（需 Go，不外发）
npm run dev     # wrangler dev（需先配 vars + secrets）
```

本地 dev：先安装此目录的 npm dependencies，再在仓库根目录运行
`scripts/ai-proxy-dev-vars.sh`。它按 `src/contracts.ts` 的 `Env` 从 Infisical dev
读取全部四个 bindings（两个 IDs、两个 secrets），原子写入 0600 的
`apps/ai-proxy/.dev.vars`。任一值缺失、重复或为空则拒绝覆盖旧文件。
这四项都需在 Infisical dev root 配齐；dashboard vars 和普通进程 env 不会自动成为本地 bindings。
模板见 `.dev.vars.example`；真实文件和写入临时文件都被 Git 忽略。

随后在仓库根目录 `process-compose up`，或 `cd apps/ai-proxy && npm run dev -- --port 8787`；
两者都使用项目内安装的 Wrangler。Infisical dev 的 `WORKER_BASE_URL` 填
`http://localhost:8787`。不需要把本机 IDs 或 secrets 写入 tracked `wrangler.toml`。

Go wire 定义统一由 `apps/api/internal/platform/workerwire` 拥有；Runtime 的 timeout、metrics 与本地 metadata 不进入 wire。`test:contract` 使用该目录的共享 request fixture，严格检查 JSON 往返不丢字段，再通过真实 Edge entry 验证 generate、零值数字、显式 mask、无图片/上传的 dry-run、实际 MIME、receipt 和全部 failure code；所有 HTTP(S) fetch 及 PUT 均使用本地 mock。普通 `go test` 不依赖 Node。

## 部署

Cloudflare Git 直连、production/preview 隔离、环境变量同步及回滚步骤统一见
[部署 runbook](../../docs/runbooks/deployment.md#ai-proxy-worker-部署cloudflare-git-直连)。

## 待首单实测确认项（P0）

1. `CF_AIG_TOKEN` 是否具备 Workers AI Read 权限（`/ai/run` 鉴权要求；仅 AI Gateway
   权限会 401）与 Unified Billing/credits 是否充足。
2. `workers-ai` 迁移到 `/ai/run` 后，`image_b64/mask_b64` 旧 body 是否被 `@cf/` 目标模型接受。
3. `airun/google` 的 `image_input` data URI 形态是否被接受（文档示例为 HTTPS URL）。
4. `airun/openai` 的 `images` data URI + `size/quality/background/output_format` 组合是否生效。
5. `airun/krea` 通用 `image_urls` data URI 是否被接受；`krea-3` 的 `resolution` 必填（`1K/2K`）
   是否需显式透传（当前仅 provider_options Merge，无缺省）。
6. `fal` 目标模型的同步可用性：若指定模型仅 queue 可用，当前 fail-closed `UNSUPPORTED_MODEL` 是否符合预期，或需换同步模型。
7. `fal` 的 `image_urls` + `mask_url` 组合是否被目标模型接受；未知字段是否被忽略或 400。
8. `gateway/openai images/edits` 经 Gateway 的 multipart 映射是否生效；`response_format: b64_json` 在 edits 下是否返回 `b64_json`。
9. `receipt.log_id`（`cf-aig-log-id` / `cf-aig-request-id`）与 `cache_status`（`cf-aig-cache-status`）
   在真实网关头是否存在（`/ai/run` 回包可能无此头）。

## Residual risk

- payload 方言按公开 docs + 基线推断，未经真生成验证；首单可能 400，需按上游报错收敛 `path_source`。
- mask 必须通过 `mask_index` 标识；省略时全部图片都是普通参考图。
- `provider_options` 后 spread 可覆盖默认 step/guidance/size；误配直接透传上游，400 归 `UPSTREAM_ERROR`。
