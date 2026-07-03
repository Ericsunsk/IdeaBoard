# Domain Docs Protocol

Use this only when a task depends on domain language, context boundaries, ADRs, or issue/refactor wording.

## Read Path

1. Read [../../DESIGN.md](../../DESIGN.md) for the context map.
2. Read the relevant local `CONTEXT.md`.
3. Read relevant ADRs under `docs/adr/` or a context-local `docs/adr/`, if they exist.

If a file does not exist, continue silently. Do not propose creating domain docs unless the task actually resolves new terms or decisions.

## Usage Rules

- Use the canonical terms from the relevant `CONTEXT.md`.
- Avoid synonyms explicitly rejected by the glossary.
- If a needed concept is missing, either avoid naming it as a domain term or note it as a domain-modeling gap.
- If your output conflicts with an existing ADR, surface the conflict instead of silently overriding it.
