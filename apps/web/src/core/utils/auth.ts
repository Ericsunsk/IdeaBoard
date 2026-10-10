import type { User } from '../../api/sessionUserType'
import { clearAuthScopedQueryCache } from '../query/queryClient'
import { resolveAuthFailureAction } from '../auth/sessionPolicy'
import type { AuthFailureAction, AuthFailureContext } from '../auth/sessionPolicy'
import { readLocalStorageItem, removeLocalStorageItem, writeLocalStorageItem } from './storage'
export {
  isSafeInternalRedirectPath,
  normalizeSafeInternalRedirectPath,
} from '../auth/sessionPolicy'
export type { AuthFailureAction, SafeInternalRedirectPath } from '../auth/sessionPolicy'

const USER_KEY = 'user'
const LAST_AUTH_STORAGE_SCOPE_KEY = 'ideaboard.auth.lastStorageScope.v1'
const authStorageScopeListeners = new Set<(scope: string | null) => void>()
let hasRedirectedToLogin = false

function notifyAuthStorageScopeChange(scope: string | null) {
  authStorageScopeListeners.forEach((listener) => listener(scope))
}

export function subscribeAuthStorageScopeChange(
  listener: (scope: string | null) => void,
): () => void {
  authStorageScopeListeners.add(listener)
  return () => authStorageScopeListeners.delete(listener)
}

export function getStoredUser(): User | null {
  const raw = readLocalStorageItem(USER_KEY)
  if (!raw) return null

  try {
    return JSON.parse(raw) as User
  } catch {
    clearStoredAuth()
    return null
  }
}

export function setStoredAuth(user: User) {
  hasRedirectedToLogin = false
  const previousScope = resolveAuthStorageUserScope(getStoredUser())
  if (!writeLocalStorageItem(USER_KEY, JSON.stringify(user))) {
    // Identity snapshot is best-effort: the in-memory session still works for
    // this tab, so warn instead of throwing the login flow off a cliff.
    console.warn('[auth] persist session snapshot failed')
  }
  const scope = resolveAuthStorageUserScope(user)
  if (scope) rememberAuthStorageUserScope(scope)
  if (scope !== previousScope) notifyAuthStorageScopeChange(scope)
}

export function clearStoredAuth() {
  removeLocalStorageItem(USER_KEY)
  notifyAuthStorageScopeChange(null)
}

type AuthStateResetListener = () => void

const authStateResetListeners = new Set<AuthStateResetListener>()

/**
 * Subscribe to silent auth-state resets (`clearStoredAuthState`). Workspace
 * delivery/stream singletons register here so background 401 paths clear
 * identity-scoped caches without `core` importing `api` (which would cycle
 * through the generated transport back into this module).
 */
export function subscribeAuthStateReset(listener: AuthStateResetListener): () => void {
  authStateResetListeners.add(listener)
  return () => {
    authStateResetListeners.delete(listener)
  }
}

function notifyAuthStateReset(): void {
  for (const listener of [...authStateResetListeners]) {
    try {
      listener()
    } catch {
      // Identity cleanup must never throw out of the auth failure path.
    }
  }
}

/**
 * Forgets the remembered device scope used for transient recovery.
 * Explicit logout calls this so the next device user cannot read the
 * previous user's locally cached boards through the recoverable scope;
 * silent expiry and 401 paths keep the remembered scope so local drafts
 * stay readable and the next login for the same user recovers them.
 */
export function forgetStoredAuthScope() {
  removeLocalStorageItem(LAST_AUTH_STORAGE_SCOPE_KEY)
}

export function clearStoredAuthState() {
  clearStoredAuth()
  clearAuthScopedQueryCache()
  notifyAuthStateReset()
}

function normalizeScopePart(value: string): string {
  return value.trim().replace(/[^a-zA-Z0-9_-]/g, '_')
}

function resolveAuthStorageUserScope(user: User | null): string | null {
  if (!user) return null

  const publicID = typeof user.public_id === 'string' ? normalizeScopePart(user.public_id) : ''
  if (publicID) {
    return `public_${publicID}`
  }

  return null
}

function rememberAuthStorageUserScope(scope: string) {
  if (readLocalStorageItem(LAST_AUTH_STORAGE_SCOPE_KEY) === scope) return
  writeLocalStorageItem(LAST_AUTH_STORAGE_SCOPE_KEY, scope, (error) => {
    console.warn('[auth] remember storage scope failed', error)
  })
}

export function getAuthStorageUserScope(): string | null {
  const scope = resolveAuthStorageUserScope(getStoredUser())
  if (scope) rememberAuthStorageUserScope(scope)
  return scope
}

export function getRecoverableAuthStorageUserScope(): string | null {
  const activeScope = getAuthStorageUserScope()
  if (activeScope) return activeScope
  const scope = readLocalStorageItem(LAST_AUTH_STORAGE_SCOPE_KEY, (error) => {
    console.warn('[auth] read recovery storage scope failed', error)
  })
  return scope?.trim() || null
}

function getCurrentAuthFailureContext(params: {
  status?: number
  payload?: unknown
  requestUrl: string
}): AuthFailureContext {
  if (typeof window === 'undefined') return params

  return {
    ...params,
    currentPathname: window.location.pathname,
    currentFullPath: window.location.pathname + window.location.search + window.location.hash,
  }
}

export function applyAuthFailureAction(action: AuthFailureAction): boolean {
  if (action.kind === 'ignore') return false

  clearStoredAuthState()
  if (action.kind === 'clear-only') return false
  if (typeof window === 'undefined') return false

  if (action.kind === 'redirect-login') {
    if (hasRedirectedToLogin) return false
    hasRedirectedToLogin = true
  }

  window.location.href = action.targetPath
  return true
}

export function handleAuthFailure(params: {
  status?: number
  payload?: unknown
  requestUrl: string
}): boolean {
  return applyAuthFailureAction(resolveAuthFailureAction(getCurrentAuthFailureContext(params)))
}

export function handleSilentAuthFailure(params: {
  status?: number
  payload?: unknown
  requestUrl: string
}): boolean {
  // Supplementary/background 401 policy: a dead session still clears local
  // auth state (so the next foreground request re-authenticates), but the
  // page is never navigated — the caller owns local error handling.
  // Always returns false (no navigation performed). Session-policy kinds
  // are reused as-is; no new kind is introduced.
  const action = resolveAuthFailureAction(getCurrentAuthFailureContext(params))
  if (action.kind === 'ignore') return false
  clearStoredAuthState()
  return false
}
