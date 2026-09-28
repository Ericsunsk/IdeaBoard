# Domain Docs Protocol

Use this only when a task depends on domain language, context boundaries, ADRs, or issue/refactor wording.

## Layout

IdeaBoard is a multi-context repository.

- `DESIGN.md` is the canonical system context map and contract.
- `apps/api/CONTEXT.md` defines Backend context language and boundaries.
- `apps/auth/CONTEXT.md` defines Auth service language (admission vs infrastructure, env contract, `{code,msg}` envelope rule) and boundaries.
- `apps/web/src/features/canvas/CONTEXT.md` defines Canvas Workspace language and boundaries.
- `docs/adr/` contains system-wide architecture decisions.
- `docs/agents/model-pool-execution-guide.md` is the execution companion for ADR-0012 and issues #847–#854.
- Context-local `docs/adr/` directories contain scoped decisions when present.

Do not create a duplicate `CONTEXT-MAP.md`; extend `DESIGN.md` when the context map changes.

## Read Path

1. Read `DESIGN.md`.
2. Read the relevant local `CONTEXT.md`.
3. Read applicable ADRs under `docs/adr/` and any context-local `docs/adr/`.

If a referenced context or ADR file does not exist, continue silently. Create domain documentation only when a task resolves new terminology or architectural decisions.

## Vocabulary Rules

- Use canonical terms from the relevant `CONTEXT.md`.
- Avoid synonyms explicitly rejected by its glossary.
- If a required concept is missing, reconsider the wording or record it as a domain-modeling gap.
- Update the relevant `CONTEXT.md` when a change intentionally alters domain language.

## ADR Conflicts

If proposed work conflicts with an existing ADR, surface the conflict explicitly instead of silently overriding it.
