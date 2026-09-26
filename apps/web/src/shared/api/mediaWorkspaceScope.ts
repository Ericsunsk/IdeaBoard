// Workspace-resolution seam for media delivery (IoC leaf, zero imports).
//
// `shared/` must not import concrete feature modules, so the workspace
// selection store is injected by `features/workspace` instead of being
// imported by `shared/api/mediaAssetDelivery`. This leaf carries no
// dependencies at all (not even transitively), so importing it never
// widens any importer's dependency closure — in particular it keeps
// `features/workspace/infrastructure/workspaceContext` light for tests
// that mock `@tanstack/vue-query`. Unset (e.g. a test that never touches
// the workspace feature) fails closed exactly like a missing selection.
let mediaWorkspaceIdResolver: (() => string | null) | null = null

export function setMediaWorkspaceIdResolver(
  resolver: (() => string | null) | null,
): void {
  mediaWorkspaceIdResolver = resolver
}

export function resetMediaWorkspaceIdResolver(): void {
  mediaWorkspaceIdResolver = null
}

export function readMediaWorkspaceId(): string | null {
  return mediaWorkspaceIdResolver?.() ?? null
}
