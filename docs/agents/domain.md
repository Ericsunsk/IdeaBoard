# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Layout

This repo uses a multi-context layout:

- `CONTEXT-MAP.md` at the repo root lists known domain contexts and points to each context's `CONTEXT.md`.
- Context-specific `CONTEXT.md` files live near the code they describe.
- `docs/adr/` is reserved for system-wide decisions.
- Context-specific ADRs may live under a context's `docs/adr/` directory.

## Before exploring, read these

- `CONTEXT-MAP.md` at the repo root, if it exists.
- Each relevant context `CONTEXT.md` listed by the map.
- `docs/adr/` ADRs that touch the area you're about to work in.
- Context-scoped ADRs under the relevant context, if present.

If any of these files don't exist, proceed silently. Don't flag their absence or suggest creating them upfront. The domain-modeling flows create them lazily when terms or decisions actually get resolved.

## Use the glossary's vocabulary

When output names a domain concept in an issue title, refactor proposal, hypothesis, or test name, use the term as defined in the relevant `CONTEXT.md`. Avoid drifting to synonyms the glossary explicitly rejects.

If the concept you need is not in the glossary yet, either reconsider whether it belongs to the project language or note it as a gap for domain modeling.

## Flag ADR conflicts

If output contradicts an existing ADR, surface it explicitly rather than silently overriding the decision.
