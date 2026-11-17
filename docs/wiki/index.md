# Wiki Index

> Agent-facing historical record. Date every claim about code; cite repo+commit. Never use "currently/now/is" without date+commit. Supersede, don't rewrite.

## Decisions

- [Wiki Location](decisions/wiki-location.md) — Wiki location, conventions, and process-document retention.
- [UI Settings Terminology](decisions/ui-settings-terminology.md) — Full unify user-visible copy to Settings (#966–#972), monitor Status EN, account recharge fix, upload truncation fix.
- [Personal Actor-Pays](decisions/personal-actor-pays.md) — Personal workspaces: payer=actor, quotas Org-only (#973–#981); 2026-11-05 worktree audit and Org quota role boundary.

## Operations

- [imgproxy 正式恢复与 Local 权限后续](operations/static-analysis-baseline.md#2026-11-17--imgproxy-正式恢复与-local-权限后续) — 2026-11-17 v0.1.39 恢复 Search / S3；Local UID 权限修复隔离验证通过，v0.1.40 因 GitHub status 检索不一致未部署，后续严格校验修复待发布。
- [imgproxy 启动失败与部署前校验](operations/static-analysis-baseline.md#2026-11-17--imgproxy-启动失败与部署前校验未提交) — 2026-11-17 只读生产日志确认 boolean typo，后续 active S3 复查另发现 sidecar secret 缺失与 AWS credential 名映射缺陷；本地修复未提交，生产写入未授权、线上未恢复（基线 `Ericsunsk/IdeaBoard@fa8f73e2a`）。
- [AI Proxy affected CI](operations/static-analysis-baseline.md#2026-11-17--ai-proxy-affected-ci-补齐) — 2026-11-17 补齐 Worker 专属验证、共享 wire/toolchain 依赖与路径选择对照；远端首次运行和成本仍待验证。
- [Verification Baselines](operations/static-analysis-baseline.md) — Dated static analysis, local CI evidence, the Auth cold-cache fix, clean-break release preparation, image security refresh, deployment topology ownership, repository-free signed deployment bundles, and 2026-11-05 worktree fingerprints / isolated DB evidence.
- [UI/UX Audit](operations/ui-ux-audit.md) — Token, component, and copy/state orphan scan; 2026-11-05 uncommitted-worktree P1/P2/P3 review (BASE c8d1b5fa1).
- [Prod Gateway Diagnosis](operations/prod-gateway-diagnosis.md) — 生产网关诊断（2026-11-03）；GWLLM protocol/BYOK、multipart 与网站审查历史，#982–#985 冻结合同修复，以及 GWLLM provider identity 修复与线上待验边界（2026-11-17，`Ericsunsk/IdeaBoard@b77a10711`）；#986/#987 Cloudflare Preview 失败诊断、仅构建验证及 clean archive 零 API 请求证据。


## 2026-11-05 — Repair follow-up (uncommitted worktree)

- [UI/UX repair verification](operations/ui-ux-audit.md#2026-11-05--p1p2p3-顺序修复与复核未提交-worktree) — Dated P1/P2/P3 fixes and verification on the dirty worktree based on `Ericsunsk/IdeaBoard@c8d1b5fa1`; supersedes the prior unresolved audit status.
- [Org quota and canonical contracts](decisions/personal-actor-pays.md#2026-11-05--org-quota-角色与-canonical-合同收敛未提交-worktree) — Dated Organization role authority, complete roster, and ADR-0042 documentation alignment.
- [Admin Copy Ownership](decisions/ui-settings-terminology.md#2026-11-05--admin-copy-ownership-恢复未提交-worktree) — Dated colocated Chinese Admin copy repair with User i18n regressions.


## 2026-11-06 — Redundancy cleanup (uncommitted worktree)

- [Ownership and behavior regressions](operations/ui-ux-audit.md#2026-11-06--冗余清理与行为回归保留未提交-worktree) — Dated Board/Quota cleanup on the dirty worktree based on `Ericsunsk/IdeaBoard@c8d1b5fa1`.
- [Verification and artifact retention](operations/static-analysis-baseline.md#2026-11-06--冗余清理验证与产物保留未提交-worktree) — Independent checks, unchanged canonical artifacts, warnings and verification limits.
- [Personal quota documentation residue](decisions/personal-actor-pays.md#2026-11-06--清理过时-personal-quota-文案未提交-worktree) — ADR-0042 alignment of the remaining Account context wording.


## 2026-11-06 — Module deepening (uncommitted worktree)

- [Member Quota and Task Readiness](operations/ui-ux-audit.md#2026-11-06--member-quota-与-task-readiness-deepening未提交-worktree) — Dated ownership and interface deepening on the uncommitted worktree, BASE `c8d1b5fa1`.
- [Deepening verification and limits](operations/static-analysis-baseline.md#2026-11-06--deepening-验证与边界未提交-worktree) — Actual frontend/backend gates, isolated DB evidence attribution, and remaining verification limits.
