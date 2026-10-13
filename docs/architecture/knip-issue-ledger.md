# Knip Governance

Knip 是当前 Frontend quality gate。Normal configuration 检查 application entrypoints；production configuration 从 Vite runtime roots 执行严格的 unused-file、dependency、export 与 duplicate analysis。

Tracked `apps/web/scripts/knipGovernanceManifest.json` 提供 reviewed public-surface annotations 和 generated-file fingerprints。`apps/web/scripts/knipGovernanceConfig.ts` 在创建 Knip configuration 前校验 manifest 与当前 Seam inventory。

在 `apps/web` 运行：

```sh
npm run lint:knip
npm run audit:knip-ledger
```

Public export、generated capability 或 named test consumer 变化时，manifest 与 source files 必须同步更新。Generated API wrappers 保持 generator-owned，不能 hand-edit。

## Current Result

Normal 与 production configuration 当前均通过。每次 `lint:knip` 重新执行实际分析，任何 non-zero exit 都是 gate failure。

<!-- KNIP_SUMMARY_START -->

| Mode       | Exit | Issues |
| ---------- | ---- | ------ |
| normal     | 0    |        |
| production | 0    |        |

<!-- KNIP_SUMMARY_END -->

## Current Issues

总计 0 个当前 issues。此表由 `audit:knip-ledger` 根据实际报告校验；更新使用 `npm run audit:knip-ledger -- --update` 并 review diff。

<!-- KNIP_ISSUES_START -->

| ID  | Mode / Category | File : Line | Symbol | Disposition | Owner | Evidence / Next Check |
| --- | --------------- | ----------- | ------ | ----------- | ----- | --------------------- |

<!-- KNIP_ISSUES_END -->

## Ownership Rules

- Public exports 由 owning feature 或 UI seam 声明。
- Test-only consumers 仅在 named contract spec 验证对应行为时获得 annotation。
- Generated capabilities 通过 source path 与 content fingerprint 保留；drift 必须通过 regeneration 修复。
- Dependency 与 boundary findings 保留在 source report 中，不使用 blanket ignore 隐藏违规。
