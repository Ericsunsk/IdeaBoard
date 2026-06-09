import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { User } from '../../shared/types'
import { getUserInfoApi } from '../../shared/api/user'
import { authClient, extractBetterAuthErrorMessage } from '../../shared/auth/client'
import {
  getStoredUser,
  setStoredAuth,
  clearStoredAuthState,
} from '../../shared/utils/auth'
import { clearAuthScopedQueryCache } from '../../shared/query/queryClient'
import { resolveAuthUrl } from '../../shared/config/runtimeUrls'

const SESSION_CACHE_TTL_MS = 30_000

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
    error && typeof error === 'object'
      ? (error as { status?: unknown; code?: unknown })
      : null
  return Number(errorRecord?.status ?? errorRecord?.code)
}

export const useUserStore = defineStore('user', () => {
  const storedUser = getStoredUser()

  const user = ref<User | null>(storedUser)
  const userInfoLoaded = ref(false)
  const sessionValidatedAt = ref(0)

  const isLoggedIn = computed(() => !!user.value)
  const isAdmin = computed(() => user.value?.role === 'admin')

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

  async function fetchUserInfo(force = false) {
    if (userInfoLoaded.value && !force) return user.value
    try {
      const res = await getUserInfoApi()
      if (res.code === 0 && res.data?.user) {
        updateAuth(res.data.user)
        return user.value
      }
    } catch (e) {
      const code = getErrorStatusCode(e)
      if (code === 401 || code === 403) {
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
