# Context Map

This repo uses a multi-context domain documentation layout. Read this file first, then open the relevant `CONTEXT.md` files for the area being changed.

## Contexts

| Context | Path | Notes |
| ------- | ---- | ----- |
| Backend | `apps/api/CONTEXT.md` | Backend context map, dependency direction, and context refactor strategy. |
| Canvas | `apps/web/src/features/canvas/CONTEXT.md` | Frontend canvas feature domain language and workflow concepts. |

## Expansion

Add new rows here when a frontend feature, backend domain, auth surface, or operational subsystem gains its own `CONTEXT.md`.

System-wide ADRs should live in `docs/adr/`. Context-specific ADRs may live beside the relevant context under `docs/adr/`.
