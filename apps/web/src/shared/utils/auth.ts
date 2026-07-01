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
  localStorage.setItem(USER_KEY, JSON.stringify(user))
}

export function clearStoredAuth() {
  localStorage.removeItem(USER_KEY)
}

export function clearStoredAuthState() {
  clearStoredAuth()
  clearAuthScopedQueryCache()
}

function normalizeScopePart(value: string): string {
  return value.trim().replace(/[^a-zA-Z0-9_-]/g, '_')
}

export function getAuthStorageUserScope(): string | null {
  const user = getStoredUser()
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
