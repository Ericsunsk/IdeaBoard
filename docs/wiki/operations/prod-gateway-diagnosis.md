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
