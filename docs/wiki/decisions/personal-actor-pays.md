# Personal Actor-Pays

Related: [Wiki Index](../index.md), [GLOSSARY](../../../GLOSSARY.md)

## 2026-11-03 — Grilled decision: personal actor-pays, quotas Org-only

- On 2026-11-03, user decided via grilling rounds: non-Default, org-less personal workspaces bill each member's own balance (payer=actor, fail-closed, no代付); personal quotas deleted; MemberQuotas narrows to Org-only; Org pool exits personal funding but Org path untouched; row-pinned settlement unchanged; no migration (greenfield, no users/data); old rules (#952-adjacent, ADR-0038 personal parts, no-fallback invariants) rewritten via explicit supersede, not silent drift. Sources: spec #973 (`ready-for-agent`), tickets #974–#981.
- Key code facts established before deciding: personal workspaces already bill owner survivor (`funds.go:270`); non-Default personal already invitable (only Default banned by #952/ADR-0039); MemberQuotas is per-(account,user) on shared money; Workspace Budget retired (#945). Scope accepted as larger-than-minimal: personal path end-to-end simplification allowed, org path and shared settlement core frozen.

## 2026-11-03 — T1 done (#974, working tree)

- On 2026-11-03, T1 landed in working tree: funds actor-path (`resolveBillingAccountRowForActor`, personal→actor survivor fail-closed, org branch untouched), `TxBillingAccountResolver` actor signature, settle_tx actor pinning + personal quota skip, 12 test/contract files (new `settle_payer_actor_test.go` red→green). Verified: build 0, workspacetasks green, related suites green, lint 0. Residual: readiness precheck still owner-based (T5), non-createInTx fallback owner-based (unreachable).

## 2026-11-03 — T7 done (#979, working tree)

- On 2026-11-03, T7 landed docs only: new ADR-0042 (accepted, payer=actor + personal no-quota, supersedes ADR-0038 §§1/3), credits/tasking CONTEXT supersede notes (English, org untouched), 0038 banners + README pointer. Residual: DESIGN §3, memberquotas/searchcatalog/tasking:139 CONTEXT left for T2–T6.

## 2026-11-03 — T3 done (#976, working tree)

- On 2026-11-03, T3 landed rename-only: personal path survivor/holder→account/owner/actor (funds, tenancy, tasking ports/store, account/workspace front copy + locale managerOnly); shared settlement Holder (TxHolder etc.) and org untouched. Verified: build/vet/tests green, locale parity 1291; full `lint` strict-seam failure is pre-existing working-tree baseline.

## 2026-11-03 — T4 done (#977, working tree)

- On 2026-11-03, T4 landed Vision payer=actor (`ResolveVisionSettlementInTx` → actor path, `Shared` flag, personal quota skip, CONTEXT rewrite). Verified: searchcatalog green (vision 18.7s), funds green, red→green Fresh DB test.

## 2026-11-03 — T6 done (#978, working tree)

- On 2026-11-03, T6 landed read-only per-actor split (billingoverview `readPersonal`, billing/usage handlers actor account, `can_manage=false`, SQL filter-first). Verified: billingoverview/billing green, funds/workspacetasks green, web billing specs 22 green. Build clean (no duplicate with T1 actor resolver).

## 2026-11-03 — T2 done (#975, working tree)

- On 2026-11-03, T2 narrowed MemberQuotas to Org-only (`ErrPersonalQuotaUnsupported` fail-closed, store/policy/read guards, HTTP 403 `billing_account_forbidden`, quote precheck `&& account.Shared`). Verified: memberquotas/tasking/v1/billingoverview/vision green, lint 0.

## 2026-11-03 — T5 done (#980, working tree)

- On 2026-11-03, T5 aligned readiness/Quote to actor account (fakes updated) and locked terminal convergence with 9 Fresh DB tests (personal 3 terminal states, post-removal convergence, no double-hold, free path, idempotency, sweep). Verified: v1/tasks green, convergence 9/9, related suites green, lint 0.

## 2026-11-03 — Series closed (#973–#981 all CLOSED, working tree uncommitted)

- On 2026-11-03, T8 added closing tests (task 4 + vision 3, cross-member isolation, insufficient/frozen fail-closed, org-closed rejection), verified build + Fresh DB suites green, then commented+closed #974–#981 and parent #973. Code stays in working tree per user constraint.

## 2026-11-04 — Personal quota card hidden (working tree, uncommitted)

- On 2026-11-04, user asked to drop the 用量额度 card in Personal billing views. `WorkspaceLimitsSection` now mounts only when `organizationId != null` (Org unchanged, locale keys shared so none removed). Verified: 2 spec files 18/18, typecheck, eslint, parity.

## 2026-11-04 — Org quota UI rebuilt (option B, working tree)

- On 2026-11-04, user chose structural rebuild: overview triplets + AdminTable member table (status badges, manager-only ops, Owner rowDisabled) + edit dialog with confirm flows; 3 new feature-local files, transport reused, zh-only colocated copy. Verified: settings specs 33/33, typecheck, eslint, parity; mounting condition untouched (Org-only).

## 2026-11-04 — Token re-audit + 3漏网 fixed (working tree)

- On 2026-11-04, user asked to re-audit/redesign with project tokens as the yardstick: 5/5 gates green, Org quota UI zero violations, previous Top10 all closed. Fixed 3 stragglers: `bg-white`→`bg-card`, white-mix named allowance (FusionStage counted in), redundant fallback removed (allowance zeroed), `rawWhiteOverlayPattern` blind spot closed + contract spec (20/20). Verified: governance 2447 green, related 88/88.

## 2026-11-04 — Org quota UI minimalism pass (working tree)- On 2026-11-04, user asked for more minimalism: overview triplets → single exhausted-only line (hidden when zero), normal rows badge-less text, hover-only row actions, footnote single-line truncate, tighter columns/headers, copy trimmed. Dialog/confirm/a11y/colocated copy/Org-only mounting untouched. Verified: settings specs 32/32, typecheck, eslint, style, parity.


## 2026-11-05 — Guest workspace toast fixed (working tree, uncommitted)

- On 2026-11-05, user reported logged-out search page always toasts "Could not load workspaces". Diagnosis: zero-request false positive — `toMenuStatus` folded `disabled` into `error`, `errorMessage` fell back to loadFailed copy, `useFailureToast` fired on mount unguarded by guest/row visibility. Fix: `disabled`→`empty` in `toMenuStatus` (login-state error paths untouched) + toast `skip` for guests/hidden rows; 2 new regression specs (red→green). Verified: new specs 4/4, related 89 passed, typecheck, eslint.

## 2026-11-05 — Org quota tokenized redesign (working tree, uncommitted)

- On 2026-11-05, user asked to redesign the Org quota block reusing project tokens. Full mapping done: colors/type/radius/shadow/spacing all already on Tier2 tokens; `leading-normal`→`leading-body`; removed the whole scoped `<style>` (`:deep` violates governance) + dead `quota-row-action` class + merged duplicate loading/error containers; zero new tokens. Hover-only row actions reverted to always-visible (simpler + accessible). Verified: specs green, typecheck, eslint, token-graph, style-governance 2449 green.

## 2026-11-05 — Quota dialog focus-ring clipping fixed (working tree, uncommitted)

- On 2026-11-05, user spotted clipped input focus rings in the member-quota dialog. Cause: dialog scrollport `overflow-hidden`/`overflow-y:auto` with zero padding clips `ring-1/2` glow. Fix (dialog-local only, shared shell untouched): form gets `p-1 -m-1` buffer. Verified: 17/17 specs, typecheck, eslint, prettier.

## 2026-11-05 — Account usage shared-account line removed (working tree, uncommitted)

- On 2026-11-05, user asked to drop the "Usage of shared account ba_…" line in the account modal usage tab. Removed the `role=status` paragraph + unused `billingAccountId` binding in `UsagePane.vue`, deleted the `account.usage.sharedAccount` key in all four locales (1291→1290), removed its spec assertion. Pure copy removal; shared-balance logic untouched. Verified: UsagePane 11/11, parity 1290, typecheck, eslint.

## 2026-11-05 — Billing shared-balance line removed (working tree, uncommitted)

- On 2026-11-05, user asked to also drop the "Shared balance account ba_…: every workspace…" line (the `billingRecords.sharedAccount` key kept in the previous pass). Removed the paragraph + unused binding in `BillingPane.vue`, deleted the key in all four locales (1290→1289 keys), removed its spec assertion. Pure copy removal; balance columns/scope logic untouched. Verified: BillingPane 9/9, parity 1289, typecheck, eslint.

## 2026-11-05 — Canvas thumbnail jitter fixed (working tree, uncommitted)

- On 2026-11-05, user reported new-canvas thumbnails jitter on appear. Causes: raster opacity hard-cut overlapping card enter, zero-size viewport first paint, enter translateY + grid without FLIP. Fix: opacity-only crossfade via motion-image token, aspect-ratio first-paint placeholder, enter opacity-only with hover mutex. Verified: thumbnail specs green, typecheck, eslint, stylelint.

## 2026-11-05 — Auto-enter thumbnail frozen (working tree, uncommitted)

- On 2026-11-05, user clarified the real issue is sequencing: during create→auto-enter, the new list card should not animate at all. Fix: `isCardEnterSuppressed` flag spans the create→openBoard handoff; `TransitionGroup :css=false` mounts the card as a still frame; manual paths keep fade-in. Verified: gallery 84 passed, browser motion 6/6, typecheck, eslint.

## 2026-11-05 — Handoff motion kill-switch via token zeroing (working tree, uncommitted)

- On 2026-11-05, per-component suppression flags removed (TransitionGroup :css binding, raster static prop, controller/contract plumbing all reverted). Replaced with a global handoff window (`documentElement.dataset.handoff`, finally+timeout cleanup) that zeroes all six motion duration tokens — no `!important`, wins by var() inheritance. Verified: gallery 85 passed, typecheck, eslint, stylelint, governance 2449 green.

## 2026-11-05 — Create auto-enter: no optimistic insert (working tree, uncommitted)

- On 2026-11-05, user judged the motion kill-switch non-standard. Reverted the whole mechanism (handoff file, motion.css block, spec asserts). Standard fix: auto-open path no longer optimistically inserts the card; trigger button shows pending, router.push on success, card adopted into store after navigation (page unmounted, nothing to animate). Return-to-list renders from warm store or mount hydrate. Verified: gallery+catalog 146 passed, typecheck, eslint, stylelint.

## 2026-11-05 — Thumbnail transparency no longer matted (working tree, uncommitted)

- On 2026-11-05, user asked transparent-background images not be matted gray. Cause: per-node frame rect (`var(--card)`) / Fusion slot (`var(--muted)`) painted under the raster, showing through transparent pixels. Fix: nodes with any possibly-alpha raster get a `--clear` (transparent) frame; JPEG/unresolved keep solid placeholder (mime sniffed server-side, fail-closed). Verified: mime + component specs green, typecheck, eslint, stylelint.

## 2026-11-05 — Thumbnail gray default flipped to transparent (working tree, uncommitted)

- On 2026-11-05, user showed one thumbnail still gray + empty nodes gray, same root cause. Diagnosis: old logic failed closed on unknown mime (pending vs ready-unknown indistinguishable), so transparent PNGs with empty mime stayed matted. Fix: readiness-based instead of mime-based — any mounted raster (any mime, JPEG covered by slice) gets transparent frame; only true pending keeps solid placeholder; empty nodes transparent with glyph only. Verified: 48 + gallery/preview 153 green, typecheck, eslint, stylelint.

## 2026-11-05 — Auto-open board missing initial document fixed (working tree, uncommitted)

- On 2026-11-05, user reported "Canvas data validation failed" on node add. Root cause was the earlier no-optimistic-insert change: `adoptAutoOpenedBoard` wrote only list meta, no document PUT/seed, so first load hydrated server `{"schema_version":1,"payload":{}}` against strict v4 schema and failed. Fix: adopt now syncs an empty v4 document (failures keep meta as `sync-failed`). H2 (If-Match 400) and H3 (stale localStorage) falsified. Verified: H1 regression 3/3 red→green, catalog+sync+gallery suites green, typecheck, eslint.


## 2026-11-05 — Actor-Pays 复审与 Org quota 边界

- 2026-11-05，对 `Ericsunsk/IdeaBoard` 未提交 worktree（BASE `c8d1b5fa11f391dd3df04620b0ac81470070cb1e`，非该 commit 本身）复审 [#973](https://github.com/Ericsunsk/IdeaBoard/issues/973)、#975/#978/#980 与 [accepted ADR-0042](../../adr/0042-personal-payer-is-actor.md)；审计 tracked diff SHA256 为 `b0492af969ec75f5c9a3ae80ca27f929f98961a146d6151f7df055490a91adba`，完整范围与验证指纹见 [Verification Baselines](../operations/static-analysis-baseline.md#2026-11-05--未提交-worktree-审计与验证边界)。
- 后端分片未发现可确认的新增 P1/P2：Task/Vision 在结算事务中解析 Personal actor 账户、Organization 共享池维持额度检查；终态按行内账户/actor 收敛，Personal 读路径先做 actor/account 过滤再聚合分页。独立临时 DB/role 的关键 Fresh DB **11/11、0 skip** 及补充 **5/5** 通过；这不构成所有并发、NATS 或 migration replay 的证明。临时 source：`/tmp/ideaboard-audit-spec.md`。
- 审计确认 **P2 UI 角色边界缺陷**：Org quota 管理依据 Organization membership，[useMemberQuotaSection.ts](../../../apps/web/src/features/account/session/quota/useMemberQuotaSection.ts) 却消费 direct Workspace roster role；合法 Org Admin 的操作可被隐藏。**P3 canonical 文档矛盾**：[MemberQuotas CONTEXT](../../../apps/api/internal/memberquotas/CONTEXT.md) 的新增 Org-only 规则与保留的 Personal/holder 段落冲突，另有 DESIGN/Workspace context 旧说明需在后续修复中核对。均未在本轮修改；source：`/tmp/ideaboard-audit-standards.md` 与 [UI/UX 复审](../operations/ui-ux-audit.md#2026-11-05--未提交-diff-的行为与规范复审)。
- 2026-11-05 的本轮真实 Router 复现限定了本页先前 “Auto-open board missing initial document fixed” 条目的验证范围：即使随后 seed schema 4，首次 Canvas GET 仍可早于 seed 并失败。该历史结论作为完整 auto-open 验收已被本轮反例 supersede；详细证据保留在上述 UI/UX 复审，本轮未修复代码。


## 2026-11-05 — Org quota 角色与 canonical 合同收敛（未提交 worktree）

- 2026-11-05，`Ericsunsk/IdeaBoard` 基于 BASE `c8d1b5fa11f391dd3df04620b0ac81470070cb1e` 的 dirty worktree 完成 P2/P3 修复；未提交。本条 **supersedes 本页“Actor-Pays 复审与 Org quota 边界”中 P2 UI 与 P3 文档仍未修复的状态**，不改写此前审计结论。规则来源继续为 [#973](https://github.com/Ericsunsk/IdeaBoard/issues/973)、[#975](https://github.com/Ericsunsk/IdeaBoard/issues/975) 与 [accepted ADR-0042](../../adr/0042-personal-payer-is-actor.md)。
- 2026-11-05，[quota controller](../../../apps/web/src/features/account/session/quota/useMemberQuotaSection.ts) 改用权威 Organization membership，与服务端 capability 一起约束 actor/target；Workspace roster role 不再收窄合法 Org Admin，也不授予 Org Member 财务权。Org/viewer 切换、读取失败与保存前权限变化 fail closed。Owner 恒 unlimited、Owner 设 Admin、Owner/Admin 设 Member、Admin 不可改 Owner/Admin 的合同保留；[component regressions](../../../apps/web/src/features/workspace/ui/settings/WorkspaceLimitsSection.component.spec.ts) 锁定管理路径及 cold Settings 4 次读取。
- 2026-11-05，后端 [ListOrganizationMembers SQL](../../../apps/api/internal/db/queries/organizations.sql) 移除任意 500 行截断，遵循 [OpenAPI](../../../apps/api/openapi.public.yaml) 的无分页完整列表合同；[502-member regression](../../../apps/api/internal/tenancy/infrastructure/postgres/organization_test.go) 旧码 red、修复后 Fresh DB green。未增加 schema、分页或授权旁路；完整列表成本 O(n)。
- 2026-11-05，[MemberQuotas CONTEXT](../../../apps/api/internal/memberquotas/CONTEXT.md) 入口、Domain Language 与 Public Seams 移除 Personal/holder quota 旧合同；[根 DESIGN §3/context map](../../../DESIGN.md) 与 [Workspace CONTEXT](../../../apps/web/src/features/workspace/CONTEXT.md) 同步 Personal payer=Actor、无额度检查/管理，Org 共享池与 role matrix 保留，终态仍按提交时行内账户与 Actor 收敛。这同时补齐本页 T7 历史记录留待后续的相关 canonical 文档缺口。
- 2026-11-05，P2 实现相关 **146 tests**、独立 **54 tests + 7 个交互场景**通过；修复方在隔离 Fresh DB 执行 **3 个顶层测试、含 subtests 共 9 项通过，0 skip**，独立复核核验原始 JSON 与测试源码，未独立重跑这些 DB cases；无 DSN 相关套件中的 **108 skip** 未计为 DB 通过。最终 frontend build、app/test typecheck、定向 lint 与严格 dependency/API surface gates 通过。完整 P1/P2/P3 结果与边界见 [UI/UX 修复记录](../operations/ui-ux-audit.md#2026-11-05--p1p2p3-顺序修复与复核未提交-worktree)；`/tmp/ideaboard-fix-quota.md`、`/tmp/ideaboard-fix-org-roster.md`、`/tmp/ideaboard-fix-docs.md` 为临时补证，不替代仓库 tests。


## 2026-11-06 — 清理过时 Personal quota 文案（未提交 worktree）

- 2026-11-06，`Ericsunsk/IdeaBoard` 基于 BASE `c8d1b5fa11f391dd3df04620b0ac81470070cb1e` 的 dirty worktree 冗余复核发现 [Account CONTEXT](../../../apps/web/src/features/account/CONTEXT.md) 仍保留 Personal owner 管理 quota 的旧片段。本轮仅纠正该片段为 Actor 自己的 Personal Billing Account、无 member quota 检查/管理；Organization membership + server capability 与 Owner/Admin/Member matrix 保留。依据为 [accepted ADR-0042 §2/§4](../../adr/0042-personal-payer-is-actor.md)，未增加 policy。
- 2026-11-06，本条补充上一轮 canonical 收敛的遗漏，不改写既有历史或声称修复已提交。[Billing Overview Reader](../../../apps/api/internal/billingoverview/read.go) 将只服务单调用链的读取生命周期收回 `Read`，保留 snapshot seam、取消后等待、错误优先级与 Personal Actor filter；Task/Vision transaction 与 row-pinned settlement 不作机械合并。[Reader 回归](../../../apps/api/internal/billingoverview/read_test.go) 与 [quota guard 回归](../../../apps/web/src/features/account/session/quota/useMemberQuotaSection.guard.unit.spec.ts) 保留，独立验证与未覆盖范围见 [Verification Baselines](../operations/static-analysis-baseline.md#2026-11-06--冗余清理验证与产物保留未提交-worktree)。
