/**
 * @deprecated P2 compatibility shim for the move to `selection/workspaceContext`.
 * TTL: 1 phase — delete after P2.
 * Deletion owner: workspace-p2. Keep account's existing import cycle-free;
 * do not add new importers.
 */
export * from '../selection/workspaceContext'
