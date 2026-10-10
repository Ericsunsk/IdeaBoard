# Shared Compatibility Context

`src/shared` contains one-phase compatibility shims after the P1b relocation. New code imports canonical destinations; no new implementation belongs here.

- [Core Context](../core/CONTEXT.md) owns horizontal primitives, auth, identity cleanup, i18n, Query placement and the Query acceptance backlog. It also defines contracts inherited by `src/theme`, `src/lib` and `src/components`.
- [API Context](../api/CONTEXT.md) owns the HTTP exit, transport contracts and generated-query policy.
- [Account Context](../features/account/CONTEXT.md) owns account UI, including `features/account/ui/chrome`.
- [Legal Context](../features/legal/CONTEXT.md) owns legal routes and public legal links.
- [Frontend Design](../../DESIGN.md) owns the UI/token contract.

Only the exact reviewed `*.shim.ts` paths may forward to feature-owned destinations. Their deletion owners and one-phase TTLs live in the canonical migration ledger; the Seam inventory declares their forwarding edges. Compatibility does not authorize new shared-to-feature dependencies.
