# Prod Gateway Providers Diagnosis

Related: [Wiki Index](../index.md), [Verification Baselines](./static-analysis-baseline.md)

## 2026-11-17 — 网站代码审查与适配范围纠正（未提交工作区）

- 2026-11-17，本轮审查以 `Ericsunsk/IdeaBoard@7988bc7d574aeccb476207ef89fd07701c50c6af` 为基线并核对未提交工作区；该 commit 不包含前两节记载的 GWLLM adapter 变更。前两节的协议与 mock 验证不能证明网站全部功能已适配，也不能证明 GWLLM 各 model/account/channel 的实际图像能力。
- 2026-11-17，代码审查将 generation、render、variations、fusion 归为可映射 Images API 的任务；工作区 [GWLLM mapper](../../../apps/ai-proxy/src/providers/gateway/openai.ts) 拒绝 mask，因而不能将依赖 mask 的功能声明为已支持。模型目录中存在 model ID 也不能替代逐 channel 的 generation、edits 和 multi-reference 验证。
- 2026-11-17，同一基线及工作区的共享链路存在网站合同缺口：[GenerationControls](../../../apps/api/internal/tasking/domain/params.go) 未定义 UI 的 `output_resolution`（2k/4k），[canonical intake](../../../apps/api/internal/tasking/application/task/canonical_intake.go) 只读取 `quality`，而 [workerImageSize](../../../apps/api/internal/runtime/interfaces/worker/providers/executor.go) 按 `Quality` 选尺寸；UI `output_count`（1–4）仍提交一次 Task，[Worker wire](../../../apps/api/internal/platform/workerwire/contract.go) 只返回单张 `image_url`。outpaint geometry 与 upscale scale 也未传入 wire；这些缺口不能归因为 GWLLM mapper 独有问题。
- 2026-11-17，同一工作区的 [Prompt engine](../../../apps/api/internal/prompts/engine.go) 已承载 variations strength 与 fusion weights 的 Prompt 合同，不能因缺少专用上游字段就判定参数丢失。[Images mapper](../../../apps/ai-proxy/src/providers/gateway/openai.ts) 允许透传 `n > 1`，但只解码 `data[0]`；这是继承 mapper 的配置风险，不是网站多结果支持的证据。
- 2026-11-17，用户纠正了首轮仅按上游协议实现、未先核对网站功能的审查范围。本轮改为先读网站源码再判断适配，仅记录上述结论；未继续编码、未做 browser 验证、未部署或执行付费请求。该记录补充并限定前两节的适用范围，不追溯宣称前轮完成了网站端到端验证。

## 2026-11-17 — GWLLM adapter protocol and verification boundary (uncommitted)

- On 2026-11-17, work based on `Ericsunsk/IdeaBoard@7988bc7d574aeccb476207ef89fd07701c50c6af` prepared an uncommitted `custom-gwllm` Image adapter. The baseline commit does not contain this adapter. Setup and capability limits are recorded in the [ai-proxy README](../../../apps/ai-proxy/README.md#gwllm-接入).
- On 2026-11-17, official [GWLLM Images API](https://doc.gwllm.com/api/images), [Cloudflare Custom Providers](https://developers.cloudflare.com/ai-gateway/configuration/custom-providers/), and [BYOK](https://developers.cloudflare.com/ai-gateway/configuration/bring-your-own-keys/) documentation supported the proposed configuration: Custom Provider slug `gwllm`, Base URL `https://api.gwllm.com`, route `custom-gwllm/v1/images/*`, provider key stored in CF BYOK. No external configuration, deployment or paid generation was performed; account/model compatibility remains a first-call verification gap.

## 2026-11-17 — GWLLM multi-reference encoding follow-up (uncommitted)

- On 2026-11-17, follow-up work on the uncommitted worktree based on `Ericsunsk/IdeaBoard@7988bc7d574aeccb476207ef89fd07701c50c6af` superseded the initial adapter's single-reference limit. The [GWLLM input mapper](../../../apps/ai-proxy/src/providers/gateway/openai.ts) preserved single-file `image` compatibility and encoded 2–4 ordered references as repeated multipart `image[]` files; mask rejection, Cloudflare sole egress and BYOK ownership remained unchanged.
- On 2026-11-17, official [GWLLM Images API](https://doc.gwllm.com/api/images) documentation mentioned `image[]` with model-specific compatibility limits; the official [OpenAI image references example](https://platform.openai.com/docs/guides/image-generation) demonstrated four repeated `image[]` files. The public [GWLLM model catalog](https://gwllm.com/api/pricing) listed GPT Image and Nano families, but its `image-generation` endpoint label did not establish edits or multi-reference support per account/channel. The adapter used the documented format without a model-name whitelist; real GPT Image/Nano multi-reference generation remained unverified, with no paid calls or external configuration performed.
- On 2026-11-17, [GWLLM regression tests](../../../apps/ai-proxy/src/providers/gateway/gwllm.test.ts) on that uncommitted worktree checked every input's bytes/order after multipart wire serialization for 2/3/4 references, four-image dry-run redaction, and rejection of `provider_options["image[]"]` role injection. The same worktree passed the TypeScript build, all 105 unit tests, and the Go→Edge wire contract; independent verification also passed the build and 105 tests. These local mock checks did not establish upstream model availability.

## 2026-11-03 — Only workers-ai on prod, no BYOK (diagnosed via browser, uncommitted)

- On 2026-11-03, user reported prod (`ideaboard.art`) gateway provider list shows only workers-ai, no BYOK. Probed with logged-in ego-browser session: `GET /api/control/v1/platform/ai-gateway/providers` → 200 with single entry `workers-ai` (Pages proxy + control auth healthy; 401 without session as expected); `POST /platform/ai-gateway/route-preview` → `valid:true, reachable:false`, message `模型标识有效（未配置 Worker，未 dry-run）`, proving prod API has no `WORKER_BASE_URL`/`WORKER_INTERNAL_SECRET`.
- With Worker discovery down (and CF discovery empty/failing), the list degrades to catalog fallback + workers-ai constant; catalog is empty on prod, so only workers-ai shows.
- Fix per runbook (`docs/runbooks/deployment.md:589`, `docs/runbooks/secrets-infisical.md`): set prod `WORKER_BASE_URL` (production ai-proxy address) + `WORKER_INTERNAL_SECRET` (+ `CF_ACCOUNT_ID`/`CF_GATEWAY_ID`/`CF_AIG_TOKEN` if BYOK via CF) in Infisical and redeploy; also verify the ai-proxy Worker itself is deployed.

## 2026-11-03 — Injection gap confirmed: /ai-proxy keys EMPTY in api container

- On 2026-11-03, `docker inspect` on prod `ideaboard-api` showed `WORKER_BASE_URL=SET` but `WORKER_INTERNAL_SECRET`/`CF_ACCOUNT_ID`/`CF_GATEWAY_ID`/`CF_AIG_TOKEN` all `EMPTY`. Root key present + whole `/ai-proxy` path missing ⇒ deploy injection did not deliver `/ai-proxy` (dual-path entrypoint verified in shipped bundle `00b826e27`; compose maps verified). Remaining suspects: VPS service token scope lacking `/ai-proxy`, or deploy used dotenvx offline fallback. Fix: grant token both paths (dashboard) or re-run entrypoint injection, then redeploy and re-inspect.

## 2026-11-03 — Redeploy with fixed token: Worker now reachable, dry-run 400

- On 2026-11-03, user replaced the prod `INFISICAL_TOKEN` (read-only, `/` + `/ai-proxy`) and re-ran `Deploy Existing Production Release` (`v0.1.32`, run `41619415622`, success). Re-probed from browser: providers list still only `workers-ai`, but route-preview message changed from `未配置 Worker，未 dry-run` to `Worker 连接失败: HTTP 400 (BAD_REQUEST)` (51ms) ⇒ `WORKER_BASE_URL`+secret now injected and Worker is being contacted; dry-run rejected (possibly fake test model id, or Worker-side validation/auth). Next: re-inspect container env (expect all SET), then read api logs for `provider discovery degraded` reason + Worker dashboard logs for `/v1/providers` and `/v1/dry-run` 4xx.

## 2026-11-03 — Root cause: CF control-plane 400, all env now SET

- On 2026-11-03, VPS showed all five keys `SET`, and api logs showed `modelregistry provider discovery degraded to catalog fallback / reason=control-plane-unavailable / err="control plane returned 400"`. So Worker/injection legs are fixed; the Cloudflare `GET .../ai-gateway/gateways/{gid}/provider_configs` call itself returns 400 (wrong gateway/account ID or token without gateway read scope). Any single-leg failure degrades the whole list to catalog fallback ⇒ only `workers-ai`. Fix on Cloudflare side (gateway existence + token scope); no redeploy needed for CF-side fixes (discovery runs per request). Worker logs not enabled — enable Workers observability for dry-run debugging.

## 2026-11-03 — CF 400 root cause: token rejected (9106)

- On 2026-11-03, full-body curl with the Infisical prod values returned `{"success":false,"errors":[{"code":9106,"message":"Authentication failed (status: 400)"}]}` ⇒ the `CF_AIG_TOKEN` stored in Infisical prod is invalid (revoked/expired/wrong-account). An earlier curl with dashboard values returned 200, so a good token exists. Fix: overwrite prod `CF_AIG_TOKEN` with the working token value, recreate the api container (values are baked at creation), then refresh the models page. No code change.

## 2026-11-03 — Resolved: BYOK providers listed

- On 2026-11-03, user overwrote prod `CF_AIG_TOKEN`, re-ran `Deploy Existing Production Release` (`v0.1.32`, run `41623503109`, success), and browser re-probe returned `fal` (available), `google-vertex-ai` (available), `openrouter` (available=false, CF-configured but Worker-unadapted, shown disabled by design), `workers-ai`. Case closed; remaining cleanup: delete misclassified `workers-ai/birefnet` and re-add under the correct BYOK provider; revoke dead tokens.
