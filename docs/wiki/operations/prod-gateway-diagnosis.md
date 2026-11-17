# Prod Gateway Providers Diagnosis

Related: [Wiki Index](../index.md), [Verification Baselines](./static-analysis-baseline.md)

## 2026-11-17 — 网站冻结执行合同修复（#982–#985）

- 2026-11-17，`Ericsunsk/IdeaBoard@04e6b05d4`（[#983](https://github.com/Ericsunsk/IdeaBoard/issues/983)）改为从冻结 Execution Plan 推导 Worker 尺寸，覆盖 1k/2k/4k，并保留旧任务和显式自定义尺寸的兼容路径。这条记录取代下方历史审查中“按 Quality 选尺寸”的未修复结论；outpaint geometry 与 upscale scale 不在本次修复范围。
- 2026-11-17，`Ericsunsk/IdeaBoard@9d88df00c`、`c72910813`（[#984](https://github.com/Ericsunsk/IdeaBoard/issues/984)）按冻结 Requested Output Count 逐张调用同一单图 Worker，各张使用独立 Output Slot，保持稳定顺序。后续上游或下一张输入签名失败保留已有结果，复用既有 Complete / CaptureSplit 按实际产物结算；首张失败及取消/超时沿用既有策略。该修复取代下方历史审查中的网站多结果缺口；Worker wire 仍为单图契约。
- 2026-11-17，`Ericsunsk/IdeaBoard@fbe2e3b20`（[#985](https://github.com/Ericsunsk/IdeaBoard/issues/985)）令 GWLLM 的 JSON/multipart 请求明确携带 numeric `n=1`，其他 `provider_options.n` 在发送前拒绝；保留 1–4 张参考图、BYOK、mask 拒绝及 preview 安全边界。该修复取代下方历史审查中“允许 n>1 但仅解码 data[0]”的配置风险结论。
- 2026-11-17，以上实现经 `Ericsunsk/IdeaBoard@39411e7e9` 整合至 [PR #986](https://github.com/Ericsunsk/IdeaBoard/pull/986)，来源为 [Spec #982](https://github.com/Ericsunsk/IdeaBoard/issues/982)。验证仅使用本地 tests / HTTP fake Worker；未访问付费上游、部署或更改线上配置。上游 model/account/channel 能力仍未经实测；系统没有上游幂等保证，崩溃重投仍可能重复付费。下方未提交工作区记录保留为历史，不代表修复后的行为。

## 2026-11-17 — 网站代码审查与适配范围纠正（未提交工作区）

- 2026-11-17，本轮审查以 `Ericsunsk/IdeaBoard@7988bc7d574aeccb476207ef89fd07701c50c6af` 为基线并核对未提交工作区；该 commit 不包含前两节记载的 GWLLM adapter 变更。前两节的协议与 mock 验证不能证明网站全部功能已适配，也不能证明 GWLLM 各 model/account/channel 的实际图像能力。
- 2026-11-17，代码审查将 generation、render、variations、fusion 归为可映射 Images API 的任务；工作区 [GWLLM mapper](../../../apps/ai-proxy/src/providers/gateway/openai.ts) 拒绝 mask，因而不能将依赖 mask 的功能声明为已支持。模型目录中存在 model ID 也不能替代逐 channel 的 generation、edits 和 multi-reference 验证。
- 2026-11-17，同一基线及工作区的共享链路存在网站合同缺口：[GenerationControls](../../../apps/api/internal/tasking/domain/params.go) 未定义 UI 的 `output_resolution`（2k/4k），[canonical intake](../../../apps/api/internal/tasking/application/task/canonical_intake.go) 只读取 `quality`，而 [workerImageSize](../../../apps/api/internal/runtime/interfaces/worker/providers/executor.go) 按 `Quality` 选尺寸；UI `output_count`（1–4）仍提交一次 Task，[Worker wire](../../../apps/api/internal/platform/workerwire/contract.go) 只返回单张 `image_url`。outpaint geometry 与 upscale scale 也未传入 wire；这些缺口不能归因为 GWLLM mapper 独有问题。
- 2026-11-17，同一工作区的 [Prompt engine](../../../apps/api/internal/prompts/engine.go) 已承载 variations strength 与 fusion weights 的 Prompt 合同，不能因缺少专用上游字段就判定参数丢失。[Images mapper](../../../apps/ai-proxy/src/providers/gateway/openai.ts) 允许透传 `n > 1`，但只解码 `data[0]`；这是继承 mapper 的配置风险，不是网站多结果支持的证据。
- 2026-11-17，用户纠正了首轮仅按上游协议实现、未先核对网站功能的审查范围。本轮改为先读网站源码再判断适配，仅记录上述结论；未继续编码、未做 browser 验证、未部署或执行付费请求。该记录补充并限定前两节的适用范围，不追溯宣称前轮完成了网站端到端验证。

## 2026-11-17 — GWLLM adapter 协议与验证边界（未提交工作区）

- 2026-11-17，基于 `Ericsunsk/IdeaBoard@7988bc7d574aeccb476207ef89fd07701c50c6af` 的未提交工作区准备了 `custom-gwllm` Image adapter；该基线 commit 尚不包含此 adapter。配置与能力边界记录于 [ai-proxy README](../../../apps/ai-proxy/README.md#gwllm-接入)。
- 2026-11-17，官方 [GWLLM Images API](https://doc.gwllm.com/api/images)、[Cloudflare Custom Providers](https://developers.cloudflare.com/ai-gateway/configuration/custom-providers/) 与 [BYOK](https://developers.cloudflare.com/ai-gateway/configuration/bring-your-own-keys/) 文档支持拟定配置：Custom Provider slug `gwllm`、Base URL `https://api.gwllm.com`、route `custom-gwllm/v1/images/*`，provider key 由 CF BYOK 保存。当轮未执行外部配置、部署或付费生成；account/model 兼容性留待实际请求验证。

## 2026-11-17 — GWLLM multi-reference 编码补充（未提交工作区）

- 2026-11-17，基于 `Ericsunsk/IdeaBoard@7988bc7d574aeccb476207ef89fd07701c50c6af` 的未提交工作区后续改动取代初版 adapter 的单参考图限制。[GWLLM input mapper](../../../apps/ai-proxy/src/providers/gateway/openai.ts) 保留单文件 `image` 兼容性，2–4 张有序参考图编码为重复 multipart `image[]` 文件；mask 拒绝、Cloudflare sole egress 与 BYOK 归属保持原约束。
- 2026-11-17，官方 [GWLLM Images API](https://doc.gwllm.com/api/images) 文档提及 `image[]` 及各 model 的兼容性限制；官方 [OpenAI image references 示例](https://platform.openai.com/docs/guides/image-generation) 展示四个重复 `image[]` 文件。公开 [GWLLM model catalog](https://gwllm.com/api/pricing) 列出 GPT Image 与 Nano families，但 `image-generation` endpoint 标签不能证明逐 account/channel 的 edits 或 multi-reference 支持。当轮 adapter 使用文档格式且未加入 model-name whitelist；真实 GPT Image/Nano multi-reference 生成未经验证，也未执行付费调用或外部配置。
- 2026-11-17，该未提交工作区的 [GWLLM regression tests](../../../apps/ai-proxy/src/providers/gateway/gwllm.test.ts) 检查 2/3/4 张参考图在 multipart wire 序列化后的 bytes/order、四图 dry-run 脱敏，以及 `provider_options["image[]"]` role injection 拒绝。当轮 TypeScript build、105 unit tests 与 Go→Edge wire contract 通过；独立验证也通过 build 与 105 tests。这些本地 mock 检查不能证明上游 model 可用性。

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

## 2026-11-17 — Cloudflare Workers PR Preview 构建失败（只读诊断）

- 2026-11-17，只读核对 [PR #987](https://github.com/Ericsunsk/IdeaBoard/pull/987)（`Ericsunsk/IdeaBoard@780004c66df240e42e09daa8d5602b5014089b39`）的 Cloudflare [Build `f66986aa-653e-4a2e-acba-b73103f57323`](https://dash.cloudflare.com/a36e696d257a3cfdaa632a2d2de01029/workers/services/view/ideaboard-ai-proxy/production/builds/f66986aa-653e-4a2e-acba-b73103f57323)：`npm run build` / `tsc --noEmit` 成功，随后 `npx wrangler preview`（Wrangler `4.156.0`）在日志时间 `11:28:42.310` 报 `Your Wrangler configuration is missing a previews block to run this command`，失败发生在 Preview deploy command，不是 TypeScript build 或新增 GitHub Worker lane。
- 2026-11-17，[PR #986](https://github.com/Ericsunsk/IdeaBoard/pull/986)（`Ericsunsk/IdeaBoard@0df605b825d6c62048e70b297b258cf92373c96c`）的 [Build `dd14b1fb-6cb7-4b1b-bda2-574e3c080a68`](https://dash.cloudflare.com/a36e696d257a3cfdaa632a2d2de01029/workers/services/view/ideaboard-ai-proxy/production/builds/dd14b1fb-6cb7-4b1b-bda2-574e3c080a68) 同样先 build 成功，再在 `11:16:46.024` 因缺少 `previews` block 失败。main `Ericsunsk/IdeaBoard@7988bc7d574aeccb476207ef89fd07701c50c6af` 的 [Build `1ebbc440-6693-42b6-9077-bdad5773e0f3`](https://dash.cloudflare.com/a36e696d257a3cfdaa632a2d2de01029/workers/services/view/ideaboard-ai-proxy/production/builds/1ebbc440-6693-42b6-9077-bdad5773e0f3) 为 success，GitHub check 含 Version ID `7616c770-ae97-4b5e-a3eb-95007af44ece`。#987 与该 main commit 的 `apps/ai-proxy` Git tree 同为 `9187156df0e4d1ba439a7230b9ea5bb8bd9612bc`；两者代码差异不能解释 Preview-only 失败。
- 2026-11-17，只读观察 [Cloudflare Build settings](https://dash.cloudflare.com/a36e696d257a3cfdaa632a2d2de01029/workers/services/view/ideaboard-ai-proxy/production/settings#builds)：production 为 `npm run build` + `npx wrangler deploy`，Preview base 为 `npm run build` + `npx wrangler preview`；二者 Root directory 均为 `apps/ai-proxy`，Preview branch builds 已开启。实际 Watch paths include 为 `*`、exclude 为空，偏离上述 main commit 对应 [deployment runbook](../../runbooks/deployment.md#ai-proxy-worker-部署cloudflare-git-直连) 约定的 `apps/ai-proxy/**`，因此只改 CI 的 #987 也触发 Worker 构建。
- 2026-11-17，修复选项尚未实施：若保留真实 PR Preview，可补 `[previews]`，但空 block 仅解除 CLI 前置检查，仍需核实 Preview base 的 dev bindings / Secrets 与 production 隔离；若只需编译验证，可将非 production 命令改为 `npx wrangler versions upload --dry-run`，但该方案不产生 Preview URL，不能等同真实 Preview 部署。runbook 的“Preview 部署，只做 dry-run 验证”也可能指仅调用 Preview 的 `/v1/dry-run`，本次未替用户决定语义。`versions upload --env preview` 仍会上传，且上述 commits 未定义 `[env.preview]`，不是可直接替换的编译 dry-run。
- 2026-11-17，在 #987 HEAD 的 clean Git archive（不含本地 `.env` / `.dev.vars`）执行 Wrangler `4.156.0` 的 `versions upload --dry-run --outdir <temporary output>`，exit `0`，输出 `Total Upload: 46.70 KiB / gzip: 10.53 KiB` 与 `--dry-run: exiting now.`；此结果只验证无上传的本地编译路径。Cloudflare Builds API 对本地 OAuth 身份返回 `403` / `12004 Forbidden`，失败日志及设置通过已登录 Dashboard 读取。本轮未修改业务代码或 Cloudflare settings、未部署或重试；未声称问题已修复、Preview Secrets 隔离或真实 Preview 可用性已验证。

## 2026-11-17 — 非 production Worker 改为仅构建验证

- 2026-11-17，用户明确选择“仅构建验证，不生成 Preview URL，不需要开发密钥”，取代上一节尚未实施的真实 Preview 候选方案。约定 Preview base Deploy command 使用 `npx wrangler versions upload --dry-run`，production 保持 `npx wrangler deploy`；不添加 `[previews]`，不新增 dev Secrets，不改 production bindings。部署合同见 [runbook](../../runbooks/deployment.md#ai-proxy-worker-部署cloudflare-git-直连)。
- 2026-11-17，Cloudflare 上述 #987 构建日志显示 `Installing nodejs 22.23.5`，与 `Ericsunsk/IdeaBoard@8bd169482db00dbcb9569d9712884fd7031c4782` 的根 `.node-version` 一致；该 commit 的 `apps/ai-proxy` 无本地 Node version 文件，Dashboard 未配置 `NODE_VERSION`。因此 Watch paths 的目标为 production / Preview base 两边 Include `apps/ai-proxy/**` 和 `.node-version`、Exclude 留空。Cloudflare 对空变更、3,000 个及以上文件或 20 个及以上 commits 的 push 有过滤例外，不能把“无关改动不触发”当绝对保证（[官方 Watch paths](https://developers.cloudflare.com/workers/ci-cd/builds/build-watch-paths/)）。
- 2026-11-17，以 `Ericsunsk/IdeaBoard@8bd169482db00dbcb9569d9712884fd7031c4782` 的 clean Git archive 执行 `npm ci --ignore-scripts`、`npm run build`，再运行锁定的 Wrangler `4.156.0` `versions upload --dry-run`，全部 exit `0`。CLI 使用临时空 HOME、无凭据环境，API base 指向本地计数服务，记录到 **0 次 API 请求**；输出 `Total Upload: 49.15 KiB / gzip: 11.08 KiB` 和 `--dry-run: exiting now.`，无 Preview URL。“Total Upload”是 bundle 大小报告，不表示发生上传。此结果只证明本地编译路径，不替代线上构建验收。
- 2026-11-17，Dashboard 保存后重新加载读回 [Build settings](https://dash.cloudflare.com/a36e696d257a3cfdaa632a2d2de01029/workers/services/view/ideaboard-ai-proxy/production/settings#builds)：production 保持 `npm run build` + `npx wrangler deploy`；Preview base 为 `npm run build` + `npx wrangler versions upload --dry-run`，Preview branch builds 开启；两边 Root directory 均为 `apps/ai-proxy`、Watch paths Include 均为 `apps/ai-proxy/**` 和 `.node-version`、Exclude 均为空。此次仅修改非 production Deploy command 与两边 Watch paths，未修改 runtime Secrets、bindings、routes，也未触发 production 部署；线上构建成功需另行核对日志。

## 2026-11-17 — v0.1.38 production 发布

- 2026-11-17，[v0.1.38](https://github.com/Ericsunsk/IdeaBoard/releases/tag/v0.1.38) 固定到 `Ericsunsk/IdeaBoard@d9cb61b3b87b8ffad803a8b60dfffda2aad9e5f5`，标准 [Docker Images run 44023385655](https://github.com/Ericsunsk/IdeaBoard/actions/runs/44023385655) 全部成功；production 部署于 05:27:36 UTC 完成，受控入口返回 `result=success / stage=complete`。API、API migrator 两张镜像重建，Auth、Auth migrator、Realtime 三张按 fingerprint 复用；签名、扫描、标准 backup、API/Auth migration 与 health/smoke 门禁完成。备份结论来自标准部署完成记录，未另行读取 VPS backup 文件。
- 2026-11-17 05:28:29 UTC，切换后独立只读复查：首页、`/api/health`（`code=0 / data.status=ok`）及 `/api/auth/get-session`（无 session，返回 `null`）均 HTTP 200。Cloudflare Pages/Worker 已独立确认运行上述 SHA，此次未另改 Cloudflare 或重复部署；未执行登录、生图或支付等业务写入验证，不据此声称上游 model 已验证。
