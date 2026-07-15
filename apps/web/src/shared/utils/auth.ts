import type { User } from '../api/generated-query/model'
import { clearAuthScopedQueryCache } from '../query/queryClient'
import { resolveAuthFailureAction } from '../auth/sessionPolicy'
import type { AuthFailureAction, AuthFailureContext } from '../auth/sessionPolicy'
export {
  isSafeInternalRedirectPath,
  normalizeSafeInternalRedirectPath,
} from '../auth/sessionPolicy'
export type { AuthFailureAction, SafeInternalRedirectPath } from '../auth/sessionPolicy'

const USER_KEY = 'user'
const LAST_AUTH_STORAGE_SCOPE_KEY = 'ideaboard.auth.lastStorageScope.v1'
const authStorageScopeListeners = new Set<(scope: string | null) => void>()

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
  const raw = localStorage.getItem(USER_KEY)
  if (!raw) return null

  try {
    return JSON.parse(raw) as User
  } catch {
    clearStoredAuth()
    return null
  }
}

export function setStoredAuth(user: User) {
  const previousScope = resolveAuthStorageUserScope(getStoredUser())
  localStorage.setItem(USER_KEY, JSON.stringify(user))
  const scope = resolveAuthStorageUserScope(user)
  if (scope) rememberAuthStorageUserScope(scope)
  if (scope !== previousScope) notifyAuthStorageScopeChange(scope)
}

export function clearStoredAuth() {
  localStorage.removeItem(USER_KEY)
  notifyAuthStorageScopeChange(null)
}

export function clearStoredAuthState() {
  clearStoredAuth()
  clearAuthScopedQueryCache()
}

function normalizeScopePart(value: string): string {
  return value.trim().replace(/[^a-zA-Z0-9_-]/g, '_')
}

function resolveAuthStorageUserScope(user: User | null): string | null {
  if (!user) return null

  const uid = typeof user.uid === 'string' ? user.uid.trim() : ''
  if (/^[A-Z0-9]{6}$/.test(uid)) {
    return `uid_${uid}`
  }

  if (typeof user.id === 'number' && Number.isFinite(user.id)) {
    return `uid_${user.id}`
  }

  const username = typeof user.username === 'string' ? normalizeScopePart(user.username) : ''
  if (username) {
    return `uname_${username}`
  }

  return null
}

function rememberAuthStorageUserScope(scope: string) {
  try {
    if (localStorage.getItem(LAST_AUTH_STORAGE_SCOPE_KEY) === scope) return
    localStorage.setItem(LAST_AUTH_STORAGE_SCOPE_KEY, scope)
  } catch (error) {
    console.warn('[auth] remember storage scope failed', error)
  }
}

export function getAuthStorageUserScope(): string | null {
  const scope = resolveAuthStorageUserScope(getStoredUser())
  if (scope) rememberAuthStorageUserScope(scope)
  return scope
}

export function getRecoverableAuthStorageUserScope(): string | null {
  const activeScope = getAuthStorageUserScope()
  if (activeScope) return activeScope
  try {
    const scope = localStorage.getItem(LAST_AUTH_STORAGE_SCOPE_KEY)
    return scope?.trim() || null
  } catch (error) {
    console.warn('[auth] read recovery storage scope failed', error)
    return null
  }
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
