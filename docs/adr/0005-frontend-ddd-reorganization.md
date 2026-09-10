# Frontend bounded-context and DDD reorganization

Status: Accepted  
Owner: Frontend Architecture  
Decision date: 2026-09-10  
Affected contexts: `board`, `canvas`, `workspace`, `account`, `admin`, `auth`, `search`, `library`, `legal`, `public-site`

We will reorganize the frontend around stable bounded contexts and use a hybrid DDD layout only where a context has real domain, application, infrastructure, and presentation responsibilities. `canvas-board` and `board-catalog` become one `board` context; `user` and `user-shell` become one `workspace` context; `canvas` remains one context with internal `domain`, `application`, `infrastructure`, `presentation`, and `tools` subdomains. `admin` remains local and does not receive a root barrel merely for symmetry.

`board` owns the persisted Board document/schema, media references, catalog CRUD, collaboration/share join, revision/sync, persistence, and previews. It has the only Board repository/sync fact source. Canvas `board-session` may orchestrate a session but must consume Board ports and must not create a second repository or sync truth source.

Internal code terminology is renamed from `user`/`user-shell` to `workspace`; public `/user/*` URLs remain unchanged because they are an external product contract. No deprecated re-exports, aliases, duplicate directories, or migration shims are allowed. Rollback is performed through Git history or branches.

The following durable and external contracts are preserved during this reorganization: persisted Board schema and migration behavior, API endpoints and request/response semantics, backend error codes, workflow feature slugs, storage keys, query invalidation semantics, public route URLs, and serialized Board/media-reference semantics. Any intentional change to one of these requires a separate migration specification and ADR.

The deliberately separate contexts are `search` and `library` (they communicate through an existing bridge/port), `admin` and `account`, `auth` and `workspace`, and `public-site` and `legal`. A future split or merge is justified only by a changed ownership invariant, an independently evolving lifecycle, a stable public API boundary, or measured dependency pressure; file count or visual similarity alone is insufficient.

Review trigger: a new durable contract, a second repository/sync owner, a cross-context deep import, or a proposed context split/merge. A later accepted ADR must supersede this record rather than silently rewriting its history.
