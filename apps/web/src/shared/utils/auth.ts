import type { User } from '../types'
import { clearAuthScopedQueryCache } from '../query/queryClient'
import {
  ADMIN_LOGIN_PATH,
  ADMIN_ROOT_PATH,
  SHARE_JOIN_PATH,
  USER_HOME_PATH,
  USER_LOGIN_PATH,
  USER_REGISTER_PATH,
  USER_ROOT_PATH,
  isPathUnderRoute,
} from '../constants/routePaths'

const USER_KEY = 'user'
const SAFE_REDIRECT_PREFIXES = [USER_ROOT_PATH, ADMIN_ROOT_PATH, SHARE_JOIN_PATH] as const
const AUTH_REDIRECT_PATHS = [USER_LOGIN_PATH, USER_REGISTER_PATH, ADMIN_LOGIN_PATH] as const
const REDIRECT_BASE_URL = 'http://ideaboard.local'

export type SafeInternalRedirectPath = {
  pathname: string
  fullPath: string
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

function getRedirectPathnameInput(path: string): string {
  const splitIndex = path.search(/[?#]/)
  return splitIndex === -1 ? path : path.slice(0, splitIndex)
}

function hasDotPathSegment(pathname: string): boolean {
  const segments = pathname.split('/')
  return segments.some((segment) => {
    try {
      const decodedSegment = decodeURIComponent(segment)
      return decodedSegment === '.' || decodedSegment === '..'
    } catch {
      return true
    }
  })
}

export function normalizeSafeInternalRedirectPath(path: string): SafeInternalRedirectPath | null {
  const redirectPath = path.trim()
  if (
    !redirectPath ||
    !redirectPath.startsWith('/') ||
    redirectPath.startsWith('//') ||
    redirectPath.includes('\\') ||
    /%5c/i.test(redirectPath)
  ) {
    return null
  }

  const inputPathname = getRedirectPathnameInput(redirectPath)
  if (hasDotPathSegment(inputPathname)) {
    return null
  }

  const baseUrl = getRedirectBaseUrl()
  let redirectUrl: URL
  try {
    redirectUrl = new URL(redirectPath, baseUrl)
  } catch {
    return null
  }

  if (redirectUrl.origin !== baseUrl) {
    return null
  }

  const redirectPathname = redirectUrl.pathname
  if (redirectPathname.includes('\\') || /%5c/i.test(redirectPathname)) {
    return null
  }

  if (!SAFE_REDIRECT_PREFIXES.some((prefix) => isPathUnderRoute(redirectPathname, prefix))) {
    return null
  }

  if (AUTH_REDIRECT_PATHS.some((authPath) => isPathUnderRoute(redirectPathname, authPath))) {
    return null
  }

  return {
    pathname: redirectPathname,
    fullPath: redirectPathname + redirectUrl.search + redirectUrl.hash,
  }
}

export function isSafeInternalRedirectPath(path: string): boolean {
  return normalizeSafeInternalRedirectPath(path) !== null
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
  return typeof window !== 'undefined' && isPathUnderRoute(window.location.pathname, ADMIN_ROOT_PATH)
}

function buildLoginRedirectPath() {
  if (typeof window === 'undefined') return USER_LOGIN_PATH
  const currentPath = window.location.pathname + window.location.search + window.location.hash
  const loginPath = isAdminPage() ? ADMIN_LOGIN_PATH : USER_LOGIN_PATH
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
    window.location.href = USER_HOME_PATH
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
