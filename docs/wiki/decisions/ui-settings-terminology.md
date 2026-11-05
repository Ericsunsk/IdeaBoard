# UI Settings Terminology

Related: [Wiki Index](../index.md), [GLOSSARY](../../../GLOSSARY.md)

## 2026-11-02 — Decision: full unify to Settings

- On 2026-11-02, user decided full unify user-visible copy to Settings (Chinese 设置, English Settings, es/ko standard translations), rejecting the page=surface / payload=configuration split. Sources: Issue #966 (spec, ready-for-agent).
- Scope frozen: code keys unchanged (settings navigation/route keys, StripeConfig API identifiers, schema). Only user-visible Chinese plus four-locale values change.
- Tickets: #967 tracer (navigation group title), #968 locale, #969 storage+search, #970 mail/OAuth/payment, #971 pricing/prompts/models, #972 close-out. All carry `Part of #966` and `ready-for-agent`.

## 2026-11-02 — Evidence

- Locale survey found 7 Chinese runtime strings in `core/i18n/locales/zh-CN` plus 60+ hardcoded Admin strings; only hard conflict was `navigation.ts:53` group title vs `navigationMeta.ts`/`routes.ts` menu/route. Sources: background survey reports filed under #966.
## 2026-11-02 — Shared copy evaluation: keep colocated

- On 2026-11-02, background evaluation for #969-#971 recommended against shared copy tokens/prefactor: Top repeats are templates with distinct nouns (e.g. 加载X失败 51/24 files, 已保存 42/20, 未配置 20/15); single-word buttons already converged via `AdminDialogActions`; `admin/CONTEXT.md:56` zh-only colocated rule plus `shared` barrel ownership make a shared strings module shallow (saves lines, not cognitive load) with higher rollback cost. Sources: grep quantification filed under #966; decision keeps #969-#971 per-vertical.
## 2026-11-02 — T1 prepared (working tree, commit withdrawn per user)

- On 2026-11-02, T1 (#967) landed as `Ericsunsk/IdeaBoard@64590b872`: `navigation.ts:53` group title to 系统设置 plus `AdminLayout.component.spec.ts` anchor sync (findAllByText length 2). Verified: AdminLayout 11/11, parity 1291 keys, typecheck, lint.
- es/ko reuse confirmed: es `configuración del sistema`, ko `시스템 설정`; en `Settings`. No locale key added (Admin title hardcoded, key freeze holds).

## 2026-11-02 — T2 prepared (working tree, commit withdrawn per user)

- On 2026-11-02, T2 (#968) landed: 4 locale values only (zh-CN admin.ts:12, board.ts:52; en counterparts Config→Settings/Setting); es/ko already standard, untouched; no spec literal to sync. Verified: parity 1291, localeParity 9/9, typecheck, lint.

## 2026-11-02 — T5 prepared (working tree, commit withdrawn per user)

- On 2026-11-02, T5 (#971) landed: 14 files (pricing/prompts/models messages + feedback + specs); 未配置→未设置一致；en Settings/Not set. Verified: 59 files 285 tests, typecheck, lint, parity; in-scope rg clean.

## 2026-11-02 — T3 prepared (working tree, commit withdrawn per user)

- On 2026-11-02, T3 (#969) landed: 10 files storage/search only (resource/feedback/messages + specs); mail/oauth/payment untouched, isolated from T4 via hunk replay. Verified: 7 files 58 tests, parity, typecheck, lint.

## 2026-11-02 — T4 prepared (working tree, commit withdrawn per user)

- On 2026-11-02, T4 (#970) landed: 14 files mail/OAuth/payment only (59+/59-); StripeConfig/API identifiers frozen. Verified: 11 files 69 tests, typecheck, lint, parity.

## 2026-11-02 — T6 prepared (working tree, commit withdrawn per user; #972 closed on tracker)

- On 2026-11-02, T6 closed residual 5 user-visible spots (profiles.ts, pages/messages supportLimit, controlGuidance, users/commands, AdminLogoPickerPopover) + 9 comment-only retains with excluded-list; 4-locale zero residue. Verified: test:run 645 files 4983 passed, typecheck, lint, build, parity 1291.

## 2026-11-02 — Monitor status column unified to English (working tree, uncommitted per user)

- On 2026-11-02, user decided monitor table status values + Status header in English (cells Pending/Running/Completed/Failed/Cancelled; header Status; other 7 headers stay Chinese; trajectory/failure/settlement copy stays Chinese). Changed `monitor/core/status.ts`, `monitor/ui/pages/messages.ts`, `TaskStatusCell` + 3 specs. Verified: 41/41 monitor specs, typecheck, eslint, parity; `lint:dependencies` strict failure pre-existing (untracked recharge spec).

## 2026-11-02 — Series closed (#966-#972 all CLOSED)

- On 2026-11-02, closer confirmed commits 64590b872/c8420aec1/ecb732fdb/8072f0d2a/a1cf2d5b9/20799933c in log; commented + closed #967-#971 and parent #966 (#972 already closed).

## 2026-11-02 — Account recharge blank fixed (working tree, uncommitted per user)

- On 2026-11-02, fixed `accountTabRegistry` recharge entry to map `active`→`isActive` (pane renamed in `1bb723e0e`, registry missed); added `RechargePane.registry.component.spec.ts` (real registry mounts real pane, asserts options load + remount reload). Verified: new spec 2/2, account 355 tests, typecheck, eslint. H4: KeepAlive remount reloads normally, no change. Billing/Usage `active` untouched. Commit withheld per user request; changes stay in working tree.

## 2026-11-03 — Upload truncation fixed at write path (working tree, uncommitted per user)

- On 2026-11-03, user reported prod uploads corrupt on both local and S3. Diagnosis: write path only ran header validation (`DecodeConfig`), so truncated bytes passed and persisted on both backends. Fix: write paths (`ingest.go`, local `StoreBytesRef`, s3 `UploadBytesWithConfigRef`, `verifyStagedUploadBytes`, `putUploadBytes` fail-fast) now use full-decode `InspectCompleteDetectedBytes`; truncated → 422, unknown format → 415. Regression tests at ingest + s3 layers (red→green verified), `go test ./internal/media/...` + v1/media green, vet/build/lint clean. Residual (intentional): streaming multipart `UploadRef` still header-checks; truncated avatar maps 503 not 422.



## 2026-11-05 — Admin Copy Ownership 恢复（未提交 worktree）

- 2026-11-05，`Ericsunsk/IdeaBoard` 基于 BASE `c8d1b5fa11f391dd3df04620b0ac81470070cb1e` 的 dirty worktree 完成 Admin Copy 修复，未提交。本条 **supersedes [UI/UX 复审](../operations/ui-ux-audit.md#2026-11-05--未提交-diff-的行为与规范复审) 中 P3 Copy“确认但未修复”的状态**；本页 2026-11-02 keep colocated 决策继续适用，[#966](https://github.com/Ericsunsk/IdeaBoard/issues/966) 的 Settings 统一未扩张为 Admin 全面 i18n。
- 2026-11-05，[ConfirmDialog](../../../apps/web/src/core/components/ConfirmDialog.vue) 的 Admin cancel/busy/unsafe-error fallback 恢复中文，User 分支继续 common locale keys；[公共 Branding validator](../../../apps/web/src/core/composables/useSiteBranding.ts) 返回最小语义错误，由既有 [settings feedback](../../../apps/web/src/features/admin/settings/session/useSettingsFeedback.ts) colocated 中文 presenter 映射。四 locale 只移除对应 7 个无用 Admin keys；allowlist 仅恢复原有 exact ConfirmDialog Admin-only 例外，不新增宽泛豁免。规则来源为 [Core Copy Policy](../../../apps/web/src/core/CONTEXT.md) 与 [Admin CONTEXT](../../../apps/web/src/features/admin/CONTEXT.md)。
- 2026-11-05，[ConfirmDialog regressions](../../../apps/web/src/core/components/ConfirmDialog.component.spec.ts) 与 [BrandingCard regressions](../../../apps/web/src/features/admin/settings/ui/cards/branding/AdminBrandingCard.component.spec.ts) 覆盖 en/es/ko 的 Admin 固定中文、User 正常翻译及实际 Save 校验；六相关套件 **58 tests**、独立四套件 **44 tests**通过，locale parity **1291 keys**、locale usage/User copy/Admin feedback gates、app/test typecheck、定向 ESLint 与最终 frontend build通过。既有 depth 5/4 与无静态引用 warn-only 未被误报为失败；未做 Browser 视觉验收。本条不取代旧条目各自当时的 locale 数量与验证范围；完整修复链见 [UI/UX 记录](../operations/ui-ux-audit.md#2026-11-05--p1p2p3-顺序修复与复核未提交-worktree)。临时补证 `/tmp/ideaboard-fix-copy.md`、`/tmp/ideaboard-verify-copy.md` 不保证跨会话保留。
