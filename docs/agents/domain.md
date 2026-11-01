# Domain Docs

## Layout and Reading Order

Use the existing root-glossary layout:

1. Read `DESIGN.md` for system contracts and the context map.
2. Read the root `GLOSSARY.md` for shared canonical names.
3. Read the relevant local `CONTEXT.md` for context-specific language and rules.
4. Read relevant decisions under `docs/adr/`.

Local `CONTEXT.md` files remain in place. Use existing context links;
do not introduce a parallel glossary map or duplicate domain definitions.

If a referenced domain document does not exist, proceed without creating
a placeholder. Record documentation only when a term or decision is resolved.

## Vocabulary and Ownership

Use canonical terms in issue titles, proposals, code, tests, and documentation.
Respect the glossary's avoided synonyms.

Record missing context-specific language in the owning `CONTEXT.md`.
Update `GLOSSARY.md` when a shared canonical name changes, keeping its
owner reference aligned. Do not duplicate local rules in the glossary.

Keep `CONTEXT.md` files in English.

## ADR Conflicts

Surface conflicts with an accepted ADR before changing its contract.
Explain the conflicting proposal and identify the affected ADR explicitly.
