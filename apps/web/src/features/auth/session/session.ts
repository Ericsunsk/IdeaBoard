import { ref, computed, reactive } from 'vue'
import type { User } from '../../../shared/api/userSession'
import { i18n } from '../../../shared/i18n'
import { authTransport } from './transport'
import { readStoredSessionUser } from './sessionPersistence'
import { createBrowserSessionTransition } from './browserSessionTransition'

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
  completeOAuthCallback: (input: {
    providerError?: unknown
    redirectPath?: unknown
    replace: (target: string | { path: string; query: Record<string, string> }) => Promise<unknown>
  }) => Promise<boolean>
  stopImpersonating: () => Promise<void>
  updatePassword: (oldPassword: string, newPassword: string) => Promise<boolean>
  setAvatarLocal: (avatar: string) => void
}

let userStore: UserSessionStore | null = null
let restoreStoredUserIfAvailable: (() => void) | null = null

function createUserSessionStore(): UserSessionStore {
  const storedUser = readStoredSessionUser()

  const user = ref<User | null>(storedUser)
  const userInfoLoaded = ref(false)
  const sessionValidatedAt = ref(0)

  const isAdmin = computed(() => user.value?.role === 'admin')

  function updateAuth(newUser: User) {
    user.value = newUser
    userInfoLoaded.value = true
    sessionValidatedAt.value = Date.now()
  }

  function resetAuth() {
    user.value = null
    userInfoLoaded.value = false
    sessionValidatedAt.value = 0
  }

  const transition = createBrowserSessionTransition({
    getUser: () => user.value,
    isLoaded: () => userInfoLoaded.value,
    getValidatedAt: () => sessionValidatedAt.value,
    commit: updateAuth,
    restore: (storedUser) => {
      user.value = storedUser
    },
    markUnloaded: () => {
      userInfoLoaded.value = false
    },
    clear: resetAuth,
  })

  async function login(identifier: string, password: string) {
    await authTransport.login(identifier, password)

    await transition.fetchUserInfo(true, { failClosedOnError: true })
    if (!user.value) {
      throw new Error(i18n.global.t('auth.errors.sessionRestoreFailed'))
    }
    return true
  }

  async function register(email: string, displayName: string, password: string) {
    await authTransport.register(email, displayName, password)
    return true
  }

  async function verifyRegistrationOTP(email: string, otp: string) {
    await authTransport.verifyRegistrationOTP(email, otp)
    await transition.fetchUserInfo(true, { failClosedOnError: true })
    if (!user.value) throw new Error(i18n.global.t('auth.errors.sessionRestoreAfterVerifyFailed'))
    return true
  }

  async function requestPasswordResetOTP(email: string) {
    await authTransport.requestPasswordResetOTP(email)
    return true
  }

  async function resetPasswordWithOTP(email: string, otp: string, password: string) {
    await authTransport.resetPasswordWithOTP(email, otp, password)
    transition.clearLocal()
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
    await transition.fetchUserInfo(true, { failClosedOnError: true })
    return true
  }

  async function logout() {
    return transition.logout()
  }

  async function updatePassword(oldPassword: string, newPassword: string) {
    await authTransport.updatePassword(oldPassword, newPassword)
    transition.clearLocal()
    return true
  }

  function setAvatarLocal(avatar: string) {
    const nextAvatar = avatar.trim()
    if (!nextAvatar || !user.value) return
    transition.setAuth({ ...user.value, avatar: nextAvatar })
  }

  const store = reactive({
    user,
    isAdmin,
    sessionValidatedAt,
    setAuth: transition.setAuth,
    clearAuth: transition.clearLocal,
    login,
    register,
    verifyRegistrationOTP,
    requestPasswordResetOTP,
    resetPasswordWithOTP,
    verifyCurrentPassword,
    requestEmailChangeOTP,
    changeEmailWithOTP,
    logout,
    fetchUserInfo: transition.fetchUserInfo,
    ensureSession: transition.ensureSession,
    completeOAuthCallback: transition.completeOAuthCallback,
    stopImpersonating: transition.stopImpersonating,
    updatePassword,
    setAvatarLocal,
  })

  restoreStoredUserIfAvailable = transition.restoreStoredUser
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
