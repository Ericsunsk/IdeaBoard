import { ref, computed, reactive } from 'vue'
import { fetchCurrentUserInfo, type User } from '../../../shared/api/userSession'
import { clearAuthScopedQueryCache } from '../../../shared/query/queryClient'
import { authTransport } from './authTransport'
import {
  clearPersistedSession,
  persistSessionUser,
  readStoredSessionUser,
} from './authSessionPersistence'

const SESSION_CACHE_TTL_MS = 30_000

export type UserSessionStore = {
  user: User | null
  readonly isAdmin: boolean
  sessionValidatedAt: number
  setAuth: (newUser: User) => void
  clearAuth: () => void
  login: (identifier: string, password: string) => Promise<boolean>
  register: (email: string, displayName: string, password: string) => Promise<boolean>
  verifyRegistrationOTP: (email: string, otp: string) => Promise<boolean>
  requestPasswordResetOTP: (email: string) => Promise<boolean>
  resetPasswordWithOTP: (email: string, otp: string, password: string) => Promise<boolean>
  verifyCurrentPassword: (password: string) => Promise<boolean>
  requestEmailChangeOTP: (newEmail: string) => Promise<boolean>
  changeEmailWithOTP: (newEmail: string, otp: string) => Promise<boolean>
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

  const publicID = typeof authUser.public_id === 'string' ? authUser.public_id.trim() : ''
  return publicID || null
}

function getErrorStatusCode(error: unknown): number {
  const errorRecord =
    error && typeof error === 'object' ? (error as { status?: unknown; code?: unknown }) : null
  return Number(errorRecord?.status ?? errorRecord?.code)
}

let userStore: UserSessionStore | null = null
let restoreStoredUserIfAvailable: (() => void) | null = null

function createUserSessionStore(): UserSessionStore {
  const storedUser = readStoredSessionUser()

  const user = ref<User | null>(storedUser)
  const userInfoLoaded = ref(false)
  const sessionValidatedAt = ref(0)

  const isAdmin = computed(() => user.value?.role === 'admin')

  function restoreStoredUser() {
    if (user.value || userInfoLoaded.value) return
    const storedUser = readStoredSessionUser()
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
    persistSessionUser(newUser)
  }

  function resetAuth() {
    user.value = null
    userInfoLoaded.value = false
    sessionValidatedAt.value = 0
    clearPersistedSession()
  }

  async function login(identifier: string, password: string) {
    await authTransport.login(identifier, password)

    await fetchUserInfo(true, { failClosedOnError: true })
    if (!user.value) {
      throw new Error('登录状态恢复失败')
    }
    return true
  }

  async function register(email: string, displayName: string, password: string) {
    await authTransport.register(email, displayName, password)
    return true
  }

  async function verifyRegistrationOTP(email: string, otp: string) {
    await authTransport.verifyRegistrationOTP(email, otp)
    await fetchUserInfo(true, { failClosedOnError: true })
    if (!user.value) throw new Error('验证成功，但登录状态恢复失败')
    return true
  }

  async function requestPasswordResetOTP(email: string) {
    await authTransport.requestPasswordResetOTP(email)
    return true
  }

  async function resetPasswordWithOTP(email: string, otp: string, password: string) {
    await authTransport.resetPasswordWithOTP(email, otp, password)
    resetAuth()
    return true
  }

  async function verifyCurrentPassword(password: string) {
    await authTransport.verifyCurrentPassword(password)
    return true
  }

  async function requestEmailChangeOTP(newEmail: string) {
    await authTransport.requestEmailChangeOTP(newEmail)
    return true
  }

  async function changeEmailWithOTP(newEmail: string, otp: string) {
    await authTransport.changeEmailWithOTP(newEmail, otp)
    await fetchUserInfo(true, { failClosedOnError: true })
    return true
  }

  async function logout() {
    const succeeded = await authTransport.logout()
    if (!succeeded) return false
    resetAuth()
    return true
  }

  async function fetchUserInfo(force = false, options: { failClosedOnError?: boolean } = {}) {
    if (userInfoLoaded.value && !force) return user.value
    try {
      const session = await fetchCurrentUserInfo()
      updateAuth(session.user)
      return user.value
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
    return authTransport.hasSession()
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
    await authTransport.updatePassword(oldPassword, newPassword)
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
    verifyRegistrationOTP,
    requestPasswordResetOTP,
    resetPasswordWithOTP,
    verifyCurrentPassword,
    requestEmailChangeOTP,
    changeEmailWithOTP,
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

export function useAuthSession() {
  return authTransport.useSession()
}

export async function startImpersonation(authUserId: string): Promise<void> {
  await authTransport.startImpersonation(authUserId)
}

export async function stopImpersonating(): Promise<void> {
  await authTransport.stopImpersonating()
}
