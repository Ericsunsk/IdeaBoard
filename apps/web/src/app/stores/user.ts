import { ref, computed, reactive } from 'vue'
import { fetchCurrentUserInfo, type User } from '../../shared/api/userSession'
import {
  authClient,
  extractBetterAuthErrorMessage,
  registerUsername,
} from '../../shared/auth/client'
import { getStoredUser, setStoredAuth, clearStoredAuthState } from '../../shared/utils/auth'
import { clearAuthScopedQueryCache } from '../../shared/query/queryClient'
import { resolveAuthUrl } from '../../shared/config/runtimeUrls'

const SESSION_CACHE_TTL_MS = 30_000

export type UserSessionStore = {
  user: User | null
  readonly isAdmin: boolean
  sessionValidatedAt: number
  setAuth: (newUser: User) => void
  clearAuth: () => void
  login: (username: string, password: string) => Promise<boolean>
  register: (username: string, password: string) => Promise<boolean>
  logout: () => Promise<boolean>
  fetchUserInfo: (
    force?: boolean,
    options?: { failClosedOnError?: boolean },
  ) => Promise<User | null>
  ensureSession: (options?: { fresh?: boolean }) => Promise<User | null>
  updatePassword: (oldPassword: string, newPassword: string) => Promise<boolean>
  setAvatarLocal: (avatar: string) => void
}

function getUserIdentity(authUser: User | null): string | null {
  if (!authUser) return null

  const authUserId = typeof authUser.auth_user_id === 'string' ? authUser.auth_user_id.trim() : ''
  if (authUserId) return `auth:${authUserId}`

  const uid = typeof authUser.uid === 'string' ? authUser.uid.trim() : ''
  if (uid) return `uid:${uid}`

  if (typeof authUser.id === 'number' && Number.isFinite(authUser.id)) {
    return `id:${authUser.id}`
  }

  const username = typeof authUser.username === 'string' ? authUser.username.trim() : ''
  if (username) return `username:${username}`

  return null
}

function getErrorStatusCode(error: unknown): number {
  const errorRecord =
    error && typeof error === 'object' ? (error as { status?: unknown; code?: unknown }) : null
  return Number(errorRecord?.status ?? errorRecord?.code)
}

let userStore: UserSessionStore | null = null
let restoreStoredUserIfAvailable: (() => void) | null = null

function createUserSessionStore(): UserSessionStore {
  const storedUser = getStoredUser()

  const user = ref<User | null>(storedUser)
  const userInfoLoaded = ref(false)
  const sessionValidatedAt = ref(0)

  const isAdmin = computed(() => user.value?.role === 'admin')

  function restoreStoredUser() {
    if (user.value || userInfoLoaded.value) return
    const storedUser = getStoredUser()
    if (storedUser) {
      user.value = storedUser
    }
  }

  function updateAuth(newUser: User) {
    if (getUserIdentity(user.value) !== getUserIdentity(newUser)) {
      clearAuthScopedQueryCache()
    }
    user.value = newUser
    userInfoLoaded.value = true
    sessionValidatedAt.value = Date.now()
    setStoredAuth(newUser)
  }

  function resetAuth() {
    user.value = null
    userInfoLoaded.value = false
    sessionValidatedAt.value = 0
    clearStoredAuthState()
  }

  async function login(username: string, password: string) {
    const result = await authClient.signIn.username({ username, password })
    if (result?.error) {
      throw new Error(extractBetterAuthErrorMessage(result.error, '登录失败'))
    }

    await fetchUserInfo(true)
    if (!user.value) {
      throw new Error('登录状态恢复失败')
    }
    return true
  }

  async function register(username: string, password: string) {
    const normalizedUsername = username.trim()

    const canonicalUsername = await registerUsername(normalizedUsername, password)
    await login(canonicalUsername, password)
    if (!user.value) {
      throw new Error('注册成功，但登录状态恢复失败')
    }
    return true
  }

  async function logout() {
    try {
      const result = await authClient.signOut()
      if (result?.error) {
        console.warn(
          '[auth] signOut failed',
          extractBetterAuthErrorMessage(result.error, '退出登录失败，请稍后重试'),
        )
        return false
      }
    } catch (error) {
      console.warn('[auth] signOut failed', error)
      return false
    }

    resetAuth()
    return true
  }

  async function fetchUserInfo(force = false, options: { failClosedOnError?: boolean } = {}) {
    if (userInfoLoaded.value && !force) return user.value
    try {
      const res = await fetchCurrentUserInfo()
      if (res.code === 0 && res.data?.user) {
        updateAuth(res.data.user)
        return user.value
      }
      if (options.failClosedOnError) {
        resetAuth()
        return null
      }
    } catch (e) {
      const code = getErrorStatusCode(e)
      if (code === 401 || code === 403) {
        resetAuth()
        return null
      }
      if (options.failClosedOnError) {
        resetAuth()
        return null
      }
      userInfoLoaded.value = false
    }
    return user.value
  }

  async function hasAuthSession() {
    try {
      const result = await authClient.getSession()
      return Boolean(result?.data?.session)
    } catch {
      return false
    }
  }

  async function ensureSession(options: { fresh?: boolean } = {}) {
    const cacheValid =
      userInfoLoaded.value &&
      !options.fresh &&
      Date.now() - sessionValidatedAt.value < SESSION_CACHE_TTL_MS
    if (cacheValid) return user.value
    if (!(await hasAuthSession())) {
      resetAuth()
      return null
    }
    return await fetchUserInfo(true, { failClosedOnError: options.fresh })
  }

  async function updatePassword(oldPassword: string, newPassword: string) {
    const response = await fetch(resolveAuthUrl('/api/auth/change-password'), {
      method: 'POST',
      credentials: 'include',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        currentPassword: oldPassword,
        newPassword,
        revokeOtherSessions: true,
      }),
    })

    if (!response.ok) {
      let errorMessage = '密码更新失败'
      try {
        const payload: unknown = await response.json()
        errorMessage = extractBetterAuthErrorMessage(payload, errorMessage)
      } catch {
        // keep fallback error message
      }
      throw new Error(errorMessage)
    }

    resetAuth()
    return true
  }

  function setAvatarLocal(avatar: string) {
    const nextAvatar = avatar.trim()
    if (!nextAvatar || !user.value) return
    updateAuth({ ...user.value, avatar: nextAvatar })
  }

  const store = reactive({
    user,
    isAdmin,
    sessionValidatedAt,
    setAuth: updateAuth,
    clearAuth: resetAuth,
    login,
    register,
    logout,
    fetchUserInfo,
    ensureSession,
    updatePassword,
    setAvatarLocal,
  })

  restoreStoredUserIfAvailable = restoreStoredUser
  return store
}

export function useUserStore() {
  if (!userStore) {
    userStore = createUserSessionStore()
    return userStore
  }

  restoreStoredUserIfAvailable?.()
  return userStore
}
