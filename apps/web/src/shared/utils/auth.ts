import type { User } from '../types'
import { clearAuthScopedQueryCache } from '../query/queryClient'

const USER_KEY = 'user'
const SAFE_REDIRECT_PREFIXES = ['/user', '/admin', '/share'] as const
const AUTH_REDIRECT_PATHS = ['/user/login', '/user/register', '/admin/login'] as const
const REDIRECT_BASE_URL = 'http://ideaboard.local'

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

function getRedirectBaseUrl(): string {
  if (typeof window === 'undefined') return REDIRECT_BASE_URL
  return window.location.origin || REDIRECT_BASE_URL
}

export function isSafeInternalRedirectPath(path: string): boolean {
  const redirectPath = path.trim()
  if (
    !redirectPath ||
    !redirectPath.startsWith('/') ||
    redirectPath.startsWith('//') ||
    redirectPath.includes('\\') ||
    /%5c/i.test(redirectPath)
  ) {
    return false
  }

  const baseUrl = getRedirectBaseUrl()
  let redirectUrl: URL
  try {
    redirectUrl = new URL(redirectPath, baseUrl)
  } catch {
    return false
  }

  if (redirectUrl.origin !== baseUrl) {
    return false
  }

  const redirectPathname = redirectUrl.pathname
  if (redirectPathname.includes('\\') || /%5c/i.test(redirectPathname)) {
    return false
  }

  if (
    !SAFE_REDIRECT_PREFIXES.some(
      (prefix) => redirectPathname === prefix || redirectPathname.startsWith(`${prefix}/`),
    )
  ) {
    return false
  }

  return !AUTH_REDIRECT_PATHS.some(
    (authPath) => redirectPathname === authPath || redirectPathname.startsWith(`${authPath}/`),
  )
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

function isAuthRequest(requestUrl: string) {
  return requestUrl.includes('/auth/')
}

function isSessionProbeRequest(requestUrl: string) {
  return requestUrl.includes('/user/info')
}

function isAdminApiRequest(requestUrl: string) {
  return requestUrl.includes('/admin/')
}

function isAdminPage() {
  return typeof window !== 'undefined' && window.location.pathname.startsWith('/admin')
}

function buildLoginRedirectPath() {
  if (typeof window === 'undefined') return '/user/login'
  const currentPath = window.location.pathname + window.location.search + window.location.hash
  const loginPath = isAdminPage() ? '/admin/login' : '/user/login'
  return loginPath + '?redirect=' + encodeURIComponent(currentPath)
}

function getPayloadCode(payload: unknown): unknown {
  return payload && typeof payload === 'object' ? (payload as { code?: unknown }).code : undefined
}

export function handleAuthFailure(params: {
  status?: number
  payload?: unknown
  requestUrl: string
}): boolean {
  const { status, payload, requestUrl } = params
  const code = getPayloadCode(payload) ?? status

  if (code === 403 && isAdminApiRequest(requestUrl) && isAdminPage()) {
    clearStoredAuthState()
    window.location.href = '/user/home'
    return true
  }

  if (code === 401 && isSessionProbeRequest(requestUrl)) {
    clearStoredAuthState()
    return false
  }

  if (code !== 401 || isAuthRequest(requestUrl)) {
    return false
  }

  clearStoredAuthState()
  if (typeof window === 'undefined') return false
  window.location.href = buildLoginRedirectPath()
  return true
}
