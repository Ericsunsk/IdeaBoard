# AI Proxy（Cloudflare Worker）

`apps/ai-proxy` 是唯一的 AI 出口适配器（Cloudflare AI Gateway sole egress 的边缘侧）。
VPS 后端只发 CanonicalImageRequest，图片输入/输出全走 presigned URL，Worker 持有网关密钥。

基线读透 `/Users/sunshikang/Desktop/脚本.ts` 后重写：不兼容旧 Go 行为，只实现最终合同。

## 冻结合同（唯一出口）

- `POST /v1/generate`，`Authorization: Bearer INTERNAL_SECRET`。
  Body `CanonicalImageRequest`：`task_id`、`provider: workers-ai|fal|openai`、`model`、`prompt`、
  `negative_prompt?`、`width?`、`height?`、`steps?`、`guidance_scale?`、`seed?`、
  `images?: string[]`（presigned GET URL，输入图在前、mask 在后）、`provider_options?`、
  `upload_url`（必填）、`public_url?`。
- 成功：`{task_id,status:completed,provider,model,image_url,content_type,seed?,duration_ms,receipt:{upstream_status,provider,model,log_id?,cache_status?}}`。
- 失败：`{task_id,status:failed,error:{code,message,upstream_status?}}`。
  queue 型（202 / `request_id` + queue status）fail-closed 报 `UNSUPPORTED_MODEL`（422），绝不轮询。
- `POST /v1/dry-run`（`upload_url` 可选）：回 `{provider,model,target:{url,method,body},path_source}`，
  headers 永不返回，body 内 `token/key/secret/auth` 全打码为 `***`，image bytes 占位。
- presigned GET 拉输入：上限 10MB + `content-type: image/*` 检查，失败 `INPUT_FETCH_FAILED`（400）。
- presigned PUT 直传输出：exact key，原样 PUT，不改写 URL；失败 `UPLOAD_FAILED`（502）。
- S3/网关密钥永不进 Worker 日志；`safeLog` 只记 `task_id/provider/model/path_source/upstream_status/duration_ms/code`。
- `INTERNAL_SECRET` 缺失即 fail-closed：任何请求都 401。

## Endpoint slug（置顶常量，见 `src/providers/airun/index.ts` 与 `src/providers/gateway/index.ts`）

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

- airun/google（nano-banana 系）：无图 text-to-image；有图 `image_input`（data URI，≤3 张）；
  `aspect_ratio` 缺省有图 `match_input_image`/无图 `1:1`，`output_format` 缺省 `png`，
  `resolution` 缺省 `1K`；`provider_options` 仅允许这三 key（超集直接 422）。
- airun/openai（gpt-image 系）：`images`（data URI）+ `quality/size/background/output_format`
  Merge 透传；`size` 缺省由 canonical width/height 合成。
- airun/krea + workers-ai 迁移：`prompt + provider_options` Merge，有图 `image_urls`（data URI，P0 待确认）。
- gateway/fal：无图 `{prompt,image_size,num_inference_steps,guidance_scale,seed}`；
  有图转 data URI（不透传 presigned URL）：`image_url` = 首张，`image_urls` = 全量（≥2 张时前 N-1），`mask_url` = 末张。
- gateway/openai：无图 `images/generations {model,prompt,size,response_format:b64_json}`；
  有图 `images/edits` multipart（`image` = 首张，`mask` = 次张，`size/response_format` 同带）。

## 环境变量

| 变量 | 说明 |
| ---- | ---- |
| `CF_ACCOUNT_ID` | Gateway account id（vars） |
| `CF_GATEWAY_ID` | Gateway id（vars） |
| `CF_AIG_TOKEN` | Gateway token（secret，`wrangler secret put`） |
| `INTERNAL_SECRET` | 后端→Worker 鉴权（secret，缺失即 401） |

## 运行与验证

```bash
cd apps/ai-proxy
npm install
npm run build   # tsc --noEmit
npm test        # providers 单测（录制 fixture，不外发）+ handler/dry-run 自测
npm run dev     # wrangler dev（需先配 vars + secrets）
```

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
- mask 语义（末张为 mask）为本仓约定，后端必须按“输入在前、mask 在后”组装 `images`。
- `provider_options` 后 spread 可覆盖默认 step/guidance/size；误配直接透传上游，400 归 `UPSTREAM_ERROR`。
