import type { User } from '../types'

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

export function hasValidStoredAuth(): boolean {
  return !!getStoredUser()
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
  const currentPath = window.location.pathname + window.location.search
  const loginPath = isAdminPage() ? '/admin/login' : '/user/login'
  return loginPath + '?redirect=' + encodeURIComponent(currentPath)
}

export function handleAuthFailure(params: {
  status?: number
  payload?: any
  requestUrl: string
}): boolean {
  const { status, payload, requestUrl } = params
  const code = payload?.code ?? status

  if (code === 403 && isAdminApiRequest(requestUrl) && isAdminPage()) {
    window.location.href = '/user/home'
    return true
  }

  if (code === 401 && isSessionProbeRequest(requestUrl)) {
    clearStoredAuth()
    return false
  }

  if (code !== 401 || isAuthRequest(requestUrl)) {
    return false
  }

  clearStoredAuth()
  window.location.href = buildLoginRedirectPath()
  return true
}
