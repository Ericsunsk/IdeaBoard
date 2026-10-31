import { fetchCurrentUserInfo } from '@/api/http/userSession'
import type { User } from '@/api/http/sessionUserType'
import { resetWorkspaceMediaDeliveryOnIdentitySwitch } from '@/api/media/mediaAssetMetadata'
import { resetWorkspaceEventStreams } from '@/api/realtime/workspaceEventStream'
import { clearAuthScopedQueryCache } from '../../../core/query/queryClient'
import { USER_BOARDS_PATH, USER_AUTH_CALLBACK_PATH } from '../../workspace/routePaths'
import { normalizeSafeInternalRedirectPath } from '../../../core/auth/sessionPolicy'
import { authTransport } from './transport'
import {
  clearPersistedSession,
  persistSessionUser,
  readStoredSessionUser,
} from './sessionPersistence'
import { forgetStoredAuthScope } from '../../../core/utils/auth'
import { isSessionFresh } from './freshness'

// Session trust window and freshness predicate live in
// `freshness.ts` (re-exported through the auth barrel); the router
// admin gate consumes the same policy instead of owning a second window.
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
  captureSession: () => () => boolean
  setAuth: (user: User) => void
  clearLocal: () => void
  fetchUserInfo: (
    force?: boolean,
    options?: { retryTransient?: boolean | undefined },
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

// Only 401/403 prove the session is dead. Network errors, timeouts, and 5xx
// are transient: the stale identity is kept so the next navigation can retry.
function isSessionExpiredStatus(status: number): boolean {
  return status === 401 || status === 403
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

  // Identity epoch: every explicit identity change (commit/clear) and every
  // fetch start claims a new generation. A fetch captures its generation and
  // commits only while it is still current, so a late resolve from a previous
  // epoch (e.g. setAuth(user-3) while a user-2 projection is in flight) can
  // neither overwrite the new identity nor clear it via a stale 401.
  let identityEpoch = 0

  // Workspace delivery/stream state is identity-scoped exactly like the Query
  // cache: a new identity must not read the previous identity's metadata, and
  // shared SSE transports must not survive the switch.
  function resetWorkspaceStateOnIdentitySwitch(): void {
    resetWorkspaceMediaDeliveryOnIdentitySwitch()
    resetWorkspaceEventStreams()
  }

  function commitUser(user: User): User {
    const previousUser = state.getUser()
    identityEpoch += 1
    state.commit(user)
    persistSessionUser(user)
    if (previousUser?.public_id !== user.public_id) {
      clearAuthScopedQueryCache()
      resetWorkspaceStateOnIdentitySwitch()
    }
    return user
  }

  function restoreStoredUser() {
    if (state.getUser() || state.isLoaded()) return
    const storedUser = readStoredSessionUser()
    if (storedUser) state.restore(storedUser)
  }

  function clearSession() {
    identityEpoch += 1
    state.clear()
    clearPersistedSession()
    clearAuthScopedQueryCache()
    resetWorkspaceStateOnIdentitySwitch()
  }

  function isStaleRequest(requestEpoch: number): boolean {
    return requestEpoch !== identityEpoch
  }

  function captureSession(): () => boolean {
    const requestEpoch = identityEpoch
    const publicId = state.getUser()?.public_id
    return () =>
      publicId != null && !isStaleRequest(requestEpoch) && state.getUser()?.public_id === publicId
  }

  function commitUnlessStale(user: User, requestEpoch: number): User | null {
    if (isStaleRequest(requestEpoch)) return state.getUser()
    return commitUser(user)
  }

  function clearUnlessStale(requestEpoch: number): User | null {
    if (isStaleRequest(requestEpoch)) return state.getUser()
    clearSession()
    return null
  }

  function failOpenUnlessStale(requestEpoch: number): User | null {
    if (isStaleRequest(requestEpoch)) return state.getUser()
    state.markUnloaded()
    return state.getUser()
  }

  async function fetchUserInfo(
    force = false,
    options: { retryTransient?: boolean | undefined } = {},
  ): Promise<User | null> {
    if (state.isLoaded() && !force) return state.getUser()
    const requestEpoch = ++identityEpoch
    try {
      const session = await loadUser()
      return commitUnlessStale(session.user, requestEpoch)
    } catch (error) {
      if (isSessionExpiredStatus(getErrorStatusCode(error))) {
        return clearUnlessStale(requestEpoch)
      }
      // Post-auth flows (login / OTP verify / email change) just proved the
      // credential, so a transient projection failure retries once before the
      // caller decides; navigation flows fail open and retry on the next nav.
      if (options.retryTransient) {
        try {
          const session = await loadUser()
          return commitUnlessStale(session.user, requestEpoch)
        } catch (retryError) {
          if (isSessionExpiredStatus(getErrorStatusCode(retryError))) {
            return clearUnlessStale(requestEpoch)
          }
        }
        return clearUnlessStale(requestEpoch)
      }
      return failOpenUnlessStale(requestEpoch)
    }
  }

  async function ensureSession(options: { fresh?: boolean } = {}): Promise<User | null> {
    const cacheValid =
      state.isLoaded() && !options.fresh && isSessionFresh(state.getValidatedAt(), now())
    if (cacheValid) return state.getUser()
    const requestEpoch = identityEpoch
    if (!(await transport.hasSession())) {
      if (requestEpoch !== identityEpoch) return state.getUser()
      clearSession()
      return null
    }
    return fetchUserInfo(true)
  }

  async function logout(): Promise<boolean> {
    const succeeded = await transport.logout()
    if (!succeeded) return false
    clearSession()
    forgetStoredAuthScope()
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
    captureSession,
    setAuth: commitUser,
    clearLocal: clearSession,
    fetchUserInfo,
    ensureSession,
    logout,
    stopImpersonating,
    completeOAuthCallback,
  }
}
