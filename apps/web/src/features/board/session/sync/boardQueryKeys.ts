// Production Board Query keys (ADR-0014 §1: source of truth).
//
// TanStack Query owns the catalog list and one document entry per board;
// the feature store owns the draft overlay. Only these two key shapes
// exist; the scope root is the invalidation prefix for stream gaps. The
// selection port owns the scope: every queryFn resolves its workspace id
// per call and fails closed before any network call when unset.

export const boardQueryKeys = {
  /** Scope root: gap invalidation prefix; never a data query itself. */
  scopeRoot: (workspaceId: string) => ['boards', workspaceId] as const,
  /** Catalog list: server board metas for one workspace. */
  list: (workspaceId: string) => ['boards', workspaceId, 'list'] as const,
  /** Per-board server document snapshot (revision + payload). */
  document: (workspaceId: string, boardId: string) =>
    ['boards', workspaceId, 'board', boardId, 'document'] as const,
}
