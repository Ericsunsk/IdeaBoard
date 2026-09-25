import { fetchCurrentUserInfo } from '../../../shared/api/userSession'
import type { User } from '../../../shared/api/sessionUser'
import { clearAuthScopedQueryCache } from '../../../shared/query/queryClient'
import { USER_BOARDS_PATH, USER_AUTH_CALLBACK_PATH } from '../../../shared/constants/routePaths'
import { normalizeSafeInternalRedirectPath } from '../../../shared/auth/sessionPolicy'
import { authTransport } from './transport'
import {
  clearPersistedSession,
  persistSessionUser,
  readStoredSessionUser,
} from './sessionPersistence'

const SESSION_CACHE_TTL_MS = 30_000

type SessionState = {
  getUser: () => User | null
  isLoaded: () => boolean
  getValidatedAt: () => number
  commit: (user: User) => void
  restore: (user: User) => void
  markUnloaded: () => void
  clear: () => void
}

type SessionTransport = Pick<typeof authTransport, 'hasSession' | 'logout' | 'stopImpersonating'>

export type BrowserSessionTransition = {
  restoreStoredUser: () => void
  setAuth: (user: User) => void
  clearLocal: () => void
  fetchUserInfo: (
    force?: boolean,
    options?: { failClosedOnError?: boolean | undefined },
  ) => Promise<User | null>
  ensureSession: (options?: { fresh?: boolean }) => Promise<User | null>
  logout: () => Promise<boolean>
  stopImpersonating: () => Promise<void>
  completeOAuthCallback: (input: {
    providerError?: unknown
    redirectPath?: unknown
    replace: (target: string | { path: string; query: Record<string, string> }) => Promise<unknown>
  }) => Promise<boolean>
}

function getErrorStatusCode(error: unknown): number {
  const errorRecord =
    error && typeof error === 'object' ? (error as { status?: unknown; code?: unknown }) : null
  return Number(errorRecord?.status ?? errorRecord?.code)
}

function getRedirectPath(redirectPath: unknown): string {
  const path = Array.isArray(redirectPath) ? redirectPath[0] : redirectPath
  if (typeof path !== 'string') return USER_BOARDS_PATH
  return normalizeSafeInternalRedirectPath(path)?.fullPath ?? USER_BOARDS_PATH
}

export function createBrowserSessionTransition(
  state: SessionState,
  dependencies: {
    transport?: SessionTransport
    loadUser?: typeof fetchCurrentUserInfo
    now?: () => number
  } = {},
): BrowserSessionTransition {
  const transport = dependencies.transport ?? authTransport
  const loadUser = dependencies.loadUser ?? fetchCurrentUserInfo
  const now = dependencies.now ?? Date.now

  function commitUser(user: User) {
    const previousUser = state.getUser()
    state.commit(user)
    persistSessionUser(user)
    if (previousUser?.public_id !== user.public_id) clearAuthScopedQueryCache()
  }

  function restoreStoredUser() {
    if (state.getUser() || state.isLoaded()) return
    const storedUser = readStoredSessionUser()
    if (storedUser) state.restore(storedUser)
  }

  function clearSession() {
    state.clear()
    clearPersistedSession()
    clearAuthScopedQueryCache()
  }

  async function fetchUserInfo(
    force = false,
    options: { failClosedOnError?: boolean | undefined } = {},
  ): Promise<User | null> {
    if (state.isLoaded() && !force) return state.getUser()
    try {
      const session = await loadUser()
      commitUser(session.user)
      return session.user
    } catch (error) {
      const status = getErrorStatusCode(error)
      if (status === 401 || status === 403 || options.failClosedOnError) {
        clearSession()
        return null
      }
      state.markUnloaded()
      return state.getUser()
    }
  }

  async function ensureSession(options: { fresh?: boolean } = {}): Promise<User | null> {
    const cacheValid =
      state.isLoaded() && !options.fresh && now() - state.getValidatedAt() < SESSION_CACHE_TTL_MS
    if (cacheValid) return state.getUser()
    if (!(await transport.hasSession())) {
      clearSession()
      return null
    }
    return fetchUserInfo(true, { failClosedOnError: options.fresh })
  }

  async function logout(): Promise<boolean> {
    const succeeded = await transport.logout()
    if (!succeeded) return false
    clearSession()
    return true
  }

  async function stopImpersonating(): Promise<void> {
    await transport.stopImpersonating()
  }

  async function completeOAuthCallback(input: {
    providerError?: unknown
    redirectPath?: unknown
    replace: (target: string | { path: string; query: Record<string, string> }) => Promise<unknown>
  }): Promise<boolean> {
    try {
      if (input.providerError) throw new Error('oauth callback failed')
      if (!(await ensureSession({ fresh: true }))) throw new Error('session provisioning failed')
      await input.replace(getRedirectPath(input.redirectPath))
      return true
    } catch {
      const loggedOut = await logout().catch(() => false)
      if (!loggedOut) clearSession()
      await input.replace({ path: USER_AUTH_CALLBACK_PATH, query: {} }).catch(() => undefined)
      return false
    }
  }

  return {
    restoreStoredUser,
    setAuth: commitUser,
    clearLocal: clearSession,
    fetchUserInfo,
    ensureSession,
    logout,
    stopImpersonating,
    completeOAuthCallback,
  }
}
