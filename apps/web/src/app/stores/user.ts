import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { User } from '../../shared/types'
import { getUserInfoApi } from '../../shared/api/user'
import { authClient, extractBetterAuthErrorMessage } from '../../shared/auth/client'
import {
  getStoredUser,
  setStoredAuth,
  clearStoredAuth,
  hasValidStoredAuth,
} from '../../shared/utils/auth'
import type { ErrorType } from '../../shared/api/orvalMutator'
import { resolveAuthUrl } from '../../shared/config/runtimeUrls'

const SESSION_CACHE_TTL_MS = 30_000

export const useUserStore = defineStore('user', () => {
  const storedUser = getStoredUser()
  const hasValidAuth = hasValidStoredAuth()

  if (!hasValidAuth && storedUser) {
    clearStoredAuth()
  }

  const user = ref<User | null>(hasValidAuth ? storedUser : null)
  const userInfoLoaded = ref(false)
  const sessionValidatedAt = ref(0)

  const isLoggedIn = computed(() => !!user.value)
  const isAdmin = computed(() => user.value?.role === 'admin')

  function updateAuth(newUser: User) {
    user.value = newUser
    userInfoLoaded.value = true
    sessionValidatedAt.value = Date.now()
    setStoredAuth(newUser)
  }

  function resetAuth() {
    user.value = null
    userInfoLoaded.value = false
    sessionValidatedAt.value = 0
    clearStoredAuth()
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
    const safeLocalPart = username
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9_-]/g, '_')
    const email = `${safeLocalPart || 'user'}@ideaboard.local`

    const result = await authClient.signUp.email({
      email,
      password,
      name: username,
      username,
    })
    if (result?.error) {
      throw new Error(extractBetterAuthErrorMessage(result.error, '注册失败'))
    }
    return true
  }

  async function logout(redirectPath = '/user/login') {
    let logoutSuccess = true
    try {
      const result = await authClient.signOut()
      if (result?.error) {
        logoutSuccess = false
      }
    } catch {
      logoutSuccess = false
    }
    resetAuth()
    const { default: router } = await import('../router')
    await router.push(redirectPath)
    return logoutSuccess
  }

  async function fetchUserInfo(force = false) {
    if (userInfoLoaded.value && !force) return user.value
    try {
      const res = await getUserInfoApi()
      if (res.code === 0 && res.data?.user) {
        updateAuth(res.data.user)
        return user.value
      }
    } catch (e) {
      const error = e as Partial<ErrorType<unknown>> & { code?: number }
      const code = Number(error.status ?? error.code)
      if (code === 401 || code === 403) {
        resetAuth()
        return null
      }
      userInfoLoaded.value = false
    }
    return user.value
  }

  async function ensureSession(options: { fresh?: boolean } = {}) {
    const cacheValid =
      userInfoLoaded.value &&
      !options.fresh &&
      Date.now() - sessionValidatedAt.value < SESSION_CACHE_TTL_MS
    if (cacheValid) return user.value
    return await fetchUserInfo(true)
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
        const payload = await response.json()
        errorMessage =
          extractBetterAuthErrorMessage(payload, errorMessage) ||
          payload?.message ||
          payload?.msg ||
          errorMessage
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

  return {
    user,
    isLoggedIn,
    isAdmin,
    setAuth: updateAuth,
    clearAuth: resetAuth,
    login,
    register,
    logout,
    fetchUserInfo,
    ensureSession,
    updatePassword,
    setAvatarLocal,
  }
})
