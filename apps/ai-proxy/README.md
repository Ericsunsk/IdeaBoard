# AI Proxy（Cloudflare Worker）

`apps/ai-proxy` 是唯一的 AI 出口适配器（Cloudflare AI Gateway sole egress 的边缘侧）。
VPS 后端只发 CanonicalImageRequest，图片输入/输出全走 presigned URL，Worker 持有网关密钥。

## 冻结合同（唯一出口）

- `POST /v1/generate`，`Authorization: Bearer WORKER_INTERNAL_SECRET`。
  Body `CanonicalImageRequest`：`task_id`、`provider: workers-ai|fal|openai|google|krea|google-vertex-ai|custom-gwllm`、`model`、`prompt`、
  `negative_prompt?`、`width?`、`height?`、`steps?`、`guidance_scale?`、`seed?`、
  `images?: string[]`（presigned GET URL）与可选 `mask_index?: number`（显式零基 mask 索引）、`provider_options?`、
  `upload_url`（必填）、`public_url?`。
  `prompt` 与 `images` 二选一必填：文生图（无 `images`）时 `prompt` 必须非空；
  有 `images`（cutout/img2img/edit）时 `prompt` 允许空串并原样透传上游。
- 成功：`{task_id,status:completed,provider,model,image_url,content_type,seed?,duration_ms,receipt:{upstream_status,provider,model,log_id?,cache_status?}}`。
- 失败：`{task_id,status:failed,error:{code,message,upstream_status?}}`。
  queue 型（202 / `request_id` + queue status）fail-closed 报 `UNSUPPORTED_MODEL`（422），绝不轮询。
- `POST /v1/dry-run`（`upload_url` 可选）：回 `{provider,model,target:{url,method,body},path_source}`，
  headers 永不返回，body 内 `token/key/secret/auth` 全打码为 `***`，image bytes 占位。
- presigned GET 拉输入：上限 10MB + `content-type: image/*` 检查，失败 `INPUT_FETCH_FAILED`（400）。
- presigned PUT 直传输出：exact key，原样 PUT，不改写 URL；失败 `UPLOAD_FAILED`（502）。Go 输出槽的 HMAC 绑定 `worker-image-v1` 固定策略（仅 PNG/JPEG/WebP），同时绑定 method/key/expiry；上传 header 必须与真实图片字节一致，API 校验 image config、大小和尺寸后按真实 MIME 存储，ingestion 再核对 Worker 回包、存储 MIME 与图片字节。槽 key 的 `.png` 后缀仅是 opaque Object Key，不决定 MIME。
- Go Worker 输出 URL 始终经 API HMAC route；Local/S3 后端共用同一验证后再写入对象，S3 读取走独立 presigned GET 并使用对象真实 MIME。未放宽浏览器 upload 的 S3 签名合同。
- 所有 Gateway transport（Fal/OpenAI 与携带 gateway id 的 `@cf/` Airun）强制 `cf-aig-skip-cache: true`、`cf-aig-collect-log-payload: false`，`provider_options` 无权覆盖。
- S3/网关密钥永不进 Worker 日志；`safeLog` 只记 `task_id/provider/model/path_source/upstream_status/duration_ms/code/message`（`message` 仅为 Worker 自产校验文案，不含上游原文/密钥）。所有 400（含校验失败）都经 `badRequest` 留痕。
- `WORKER_INTERNAL_SECRET` 缺失即 fail-closed：任何请求都 401。
- `GET /v1/providers`（需 `Authorization: Bearer WORKER_INTERNAL_SECRET`，与 generate/dry-run 一致；无 task 上下文，401 body 为 `{error:{code,message}}` 不带 `task_id`）：回 `{providers: string[]}`，例如 `{"providers":["fal","google","google-vertex-ai","krea","openai","workers-ai"]}`。列表从 `src/providers/index.ts` 的 `PROVIDER_REGISTRY` key（`${channel}:${vendor}`）派生 vendor 去重排序，内部 vendor `migrated` 对外为 `workers-ai`，新增 vendor 加表项即自动出现（见 `listProviders`；VPS 侧不硬编码名单）。

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
  `openai/images/edits`（有图 multipart）、`google-vertex-ai/v1/projects/{project}/locations/{region}/publishers/google/models/{model}:{resource}`
  （Vertex 原生端点；`project/region/resource` 来自 `provider_options`
  `vertex_project`（必填）/`vertex_region`（缺省 `us-central1`）/`vertex_resource`
  （缺省 `generateContent`，仅接受 `generateContent` 或 `predict` 前缀族）；
  网关鉴权恒为 `cf-aig-authorization: Bearer CF_AIG_TOKEN`，
  BYOK/直连 Google `Authorization` 场景首单待确认，默认不带）。
- gateway/custom-gwllm：Cloudflare Custom Provider（slug `gwllm`、Base URL
  `https://api.gwllm.com`），请求经 `custom-gwllm/v1/images/generations`（无图 JSON）
  或 `custom-gwllm/v1/images/edits`（1–4 张参考图 multipart）；复用 OpenAI Images 字段与回包解析。

GET Probe（如 `/v1/models`）验不出计费/模型可用性与 payload 方言，必须首单真生成确认。

## Image 映射（首单待确认见下）

图片角色合同：`images` 保留普通参考图顺序（最多 4 张普通图）；可选 `mask_index` 是其中 mask 的零基索引，mask 另占一张，且必须至少有一张普通输入图。不传表示全部为普通参考图。Fal 保留全部参考图，仅显式 mask 映射为 `mask_url`；Gateway OpenAI edits 与 migrated Workers AI 只支持一张普通输入加可选 mask，多参考图明确拒绝。GWLLM edits 保留最多 4 张普通参考图，拒绝显式 mask。Airun Google/OpenAI/Krea 保留普通参考图，但目前拒绝显式 mask。dry-run 使用相同校验和角色映射。 `provider_options` 不允许注入图片或 mask 角色字段。

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
- gateway/custom-gwllm：无图 JSON 文生图；单参考图 multipart `image`；2–4 张参考图
  按 canonical 顺序使用重复的 multipart `image[]` 文件字段，上传已拉取的原始图片字节，
  不透传 presigned URL；不支持 mask。具体模型/渠道是否接受多图须首单确认。
- gateway/google-vertex-ai（Vertex 原生，provider slug 精确匹配 `google-vertex-ai`，
  禁止 `google-vertex` 等变体）：`generateContent` 走
  `{contents:[{role:"user",parts:[{text}, ...inlineData]}]}`（`prompt` 必填，
  无 `prompt` 即使有图也 fail-closed；图片为已拉取字节的 `inlineData{mimeType,data}`，
  绝不透传 presigned URL）；`predict`（Imagen）走 `{instances:[{prompt,...}],parameters}`
  （`parameters` 全部来自显式 `provider_options`，不猜缺省；canonical `seed/width/height`
  如有则透传；`vertex_project/region/resource` 保留键不进 `parameters`）。
  回包：`generateContent` 取 `candidates[0].content.parts` 内首个 `inlineData` 图片
  （纯文本无图即 `UPSTREAM_ERROR`）；`predict` 取 `predictions[0]` 的顶层/嵌套
  `bytesBase64Encoded`（取不到即 `UPSTREAM_ERROR`，绝不猜）。

## GWLLM 接入

1. 在现有 Cloudflare AI Gateway 创建 Custom Provider：slug `gwllm`，Base URL
   `https://api.gwllm.com`（不带 `/v1`）；在该 Provider 的 BYOK 中保存 GWLLM API key，
   使用 `default` alias。Worker 只发送 `cf-aig-authorization`，由 Gateway 注入上游
   `Authorization`；不新增 Worker Secret，不把 GWLLM key 放入 `provider_options`。
2. 将此 Worker 版本部署后，在 Admin Model Catalog 选择 `custom-gwllm`，填写该 API key
   可见的精确 model ID（保留大小写、后缀及其自身 namespace），再按需加入 Service Model Pool。
   不根据 OpenAI model 名猜测 GWLLM 可用模型；模型可见性以 GWLLM 控制台或 `/v1/models` 为准。
3. Route Preview 仅 dry-run；实际计费、模型可用性及图片参数仍需操作员确认后做首单真生成。

Adapter 能力范围：同步文生图、1–4 张参考图 multipart 编辑（2–4 张用于多图融合）；
明确拒绝 mask、超过 4 张参考图、stream，以及 `provider_options` 覆盖 canonical
`model/prompt` 或图片角色（包括 `image[]`）。每次 Worker 请求只产生一张 Task Output，
明确发送 `n: 1`；`provider_options.n` 如有配置，必须为数字 `1`，其他值在调用上游前拒绝。
网站 `output_count` 由 Go 逐张调用 Worker，不映射成 GWLLM 的批量 `n`。

文生图与编辑使用相同字段规则：canonical `width/height` 任一存在时，`size` 由该尺寸
生成（另一维缺省 `1024`），优先于 `provider_options.size`；没有 canonical 尺寸时，
允许显式 `provider_options.size`，缺省 `1024x1024`。
`response_format` 均取 `provider_options.response_format`，缺省 `b64_json`；
回包接受 `data[0].b64_json` 或 `data[0].url`。`quality` 等其他参数按目标模型/渠道
支持情况透传；字段兼容性待首单确认。multipart dry-run 展示实际序列化字段，图片字节
使用占位符，敏感字段（含嵌套对象）脱敏；预览不验证供应商是否接受这些字段。
异步/队列回包 fail-closed，不轮询、不新增重试。

可复制的 Worker dry-run body 模板（将 `model` 占位符替换为自己账号可见的精确 model ID；
不包含任何密钥，不需要上传槽）：

```json
{
  "task_id": "gwllm-preview",
  "provider": "custom-gwllm",
  "model": "<EXACT_MODEL_ID_FROM_YOUR_GWLLM_ACCOUNT>",
  "prompt": "A bicycle",
  "width": 2048,
  "height": 2048,
  "provider_options": {
    "n": 1,
    "response_format": "b64_json"
  }
}
```

Admin Model Catalog 的 Provider 选择 `custom-gwllm`，Model 使用相同的精确 ID；
Model Pool 的 Custom Params 可从 `{"n":1,"response_format":"b64_json"}` 开始。
网站按用户选择生成 canonical 尺寸，无需在 Custom Params 重复填写 `size`；上例只展示
字段映射，不承诺该 model/channel 支持 `2048x2048`。`quality`、特殊 `size` 或其他模型参数
应以该账号渠道的协议为准，不根据 GPT Image/Nano 名称自动补参数。

GPT Image 与 Google Nano 模型均通过此 Images API adapter 使用，不按名称改路由或截断参考图。
2026-11-17 的 GWLLM 公开模型目录列出 `gpt-image-2.6-flare`、`gpt-image-2.6-sunburst`、
`nano-banana-pro`、`nano-banana-2.1` 及各自的 `-2k` / `-4k` 变体；此目录不证明各模型
或账号渠道支持 edits/多图。GWLLM 通用文档提及 `image[]`，OpenAI 官方提供四图编辑示例；
本地测试验证 2/3/4 张图逐张完整编码及顺序，实际可用性仍须按精确 model ID 与渠道首单确认。

协议来源：[GWLLM Images API](https://doc.gwllm.com/api/images)、
[GWLLM 公开模型目录](https://gwllm.com/api/pricing)、
[OpenAI Image references 示例](https://platform.openai.com/docs/guides/image-generation)、
[Cloudflare Custom Providers](https://developers.cloudflare.com/ai-gateway/configuration/custom-providers/)、
[Cloudflare BYOK](https://developers.cloudflare.com/ai-gateway/configuration/bring-your-own-keys/)。
此接入仅完成本地 Mock 验证，未创建线上 Provider/BYOK、未部署、未执行付费生成。

## 环境变量

| 变量                     | 说明                                                                                  |
| ------------------------ | ------------------------------------------------------------------------------------- |
| `CF_ACCOUNT_ID`          | Gateway account id（线上 vars；本地从 Infisical dev `/ai-proxy` 写入 `.dev.vars`）    |
| `CF_GATEWAY_ID`          | Gateway id（线上 vars；本地从 Infisical dev `/ai-proxy` 写入 `.dev.vars`）            |
| `CF_AIG_TOKEN`           | Gateway token（线上 Secret；本地仅 `.dev.vars`，源为 dev `/ai-proxy`）                |
| `WORKER_INTERNAL_SECRET` | 后端→Worker 鉴权（线上 Secret；本地仅 `.dev.vars`，源为 dev `/ai-proxy`；缺失即 401） |

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
`/ai-proxy` folder 读取全部四个 bindings（两个 IDs、两个 secrets），原子写入 0600 的
`apps/ai-proxy/.dev.vars`。任一值缺失、重复或为空则拒绝覆盖旧文件。
这四项都需在 Infisical dev `/ai-proxy` 配齐（root `/` 不再是来源；root 残留的
`WORKER_INTERNAL_SECRET` 副本待用户在 dashboard 手动删除，删除前灌脚本每次以
`/ai-proxy` 为准做哈希一致性校验，不一致则拒绝写入）；dashboard vars 和普通进程 env 不会自动成为本地 bindings。
模板见 `.dev.vars.example`；真实文件和写入临时文件都被 Git 忽略。

随后在仓库根目录 `process-compose up`，或 `cd apps/ai-proxy && npm run dev -- --port 8787`；
两者都使用项目内安装的 Wrangler。Infisical dev 的 `WORKER_BASE_URL` 填
`http://localhost:8787`。不需要把本机 IDs 或 secrets 写入 tracked `wrangler.toml`。

dev 无需加白：VPS→Worker 走 Configured 端点直通（`ParseConfiguredEndpoint` +
`RequestOptions.Configured`，loopback 直接放行），不经过出站 allowlist；
`api_outbound_allow_hosts` 为空即正常工作，出站策略只管用户可影响的 URL。

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
10. `gateway/google-vertex-ai` 首单真生成确认：`predict` 回包形态（顶层 vs
    `image.bytesBase64Encoded` 嵌套以实测为准，当前双兼容）；`vertex_region` 缺省
    `us-central1` 对目标模型的可用性；直连 Google `Authorization`（BYOK）场景是否需要、
    与 `cf-aig-authorization` 并存时的头优先级；`generateContent` 是否需
    `generationConfig.responseModalities` 等额外字段（当前为最小 `contents`，未猜）。

## Residual risk

- payload 方言按公开 docs + 基线推断，未经真生成验证；首单可能 400，需按上游报错收敛 `path_source`。
- mask 必须通过 `mask_index` 标识；省略时全部图片都是普通参考图。
- 各 provider 的字段规则分别生效；GWLLM 的 canonical 尺寸与单张输出约束见上文。
  其他 provider 的 `provider_options` 可覆盖其默认 step/guidance/size；误配透传上游，400 归 `UPSTREAM_ERROR`。
