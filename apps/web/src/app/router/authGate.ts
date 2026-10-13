import { isSessionFresh } from '../../features/auth'
import { buildLoginRedirectPath } from '../../core/auth/sessionPolicy'
import { WORKSPACE_DEFAULT_PATH } from '../../features/workspace/routes/workspaceRoutes'

export type AuthSessionRefreshDecision = {
  shouldRefresh: boolean
  needsFresh: boolean
}

/**
 * Pure router session-refresh decision. Freshness itself is owned by the auth
 * session policy (`isSessionFresh`); this module only composes it with route
 * metadata. `now` is injectable so freshness boundaries and repeated
 * navigations are verifiable without wall-clock flakiness.
 */
export function resolveAuthSessionRefresh(input: {
  isAdminRoute: boolean
  requiresAuth?: boolean | undefined
  guest?: boolean | undefined
  validatedAt: number
  now?: number | undefined
}): AuthSessionRefreshDecision {
  // Admin routes revalidate at most every trust window: a recently validated
  // session is trusted between navigations instead of reprobing on every
  // admin navigation.
  const needsFresh =
    input.isAdminRoute && !isSessionFresh(input.validatedAt, input.now ?? Date.now())
  const shouldRefresh = needsFresh || Boolean(input.requiresAuth) || Boolean(input.guest)
  return { shouldRefresh, needsFresh }
}

export type AuthGateUser = {
  role?: string | null | undefined
} | null

/**
 * Pure redirect decision once the session user for this navigation is known
 * (cached or freshly revalidated). Returns the redirect path or `true` to
 * continue the navigation.
 */
export function resolveAuthGateRedirect(input: {
  isAdminRoute: boolean
  requiresAuth?: boolean | undefined
  guest?: boolean | undefined
  user: AuthGateUser
  currentPath: string
  currentFullPath: string
}): string | true {
  const isLoggedIn = Boolean(input.user)

  if ((input.requiresAuth || input.isAdminRoute) && !isLoggedIn) {
    return buildLoginRedirectPath(input.currentPath, input.currentFullPath)
  }

  if (input.guest && isLoggedIn) {
    return WORKSPACE_DEFAULT_PATH
  }

  if (input.isAdminRoute && input.user && input.user.role !== 'admin') {
    return WORKSPACE_DEFAULT_PATH
  }

  return true
}
