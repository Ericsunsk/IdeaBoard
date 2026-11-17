# Prod Gateway Providers Diagnosis

Related: [Wiki Index](../index.md), [Verification Baselines](./static-analysis-baseline.md)

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
