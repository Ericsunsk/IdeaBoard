import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { User } from '../../shared/types'
import { userApi } from '../../shared/api/user'
import { authClient, extractBetterAuthErrorMessage } from '../../shared/auth/client'
import {
  getStoredUser,
  setStoredAuth,
  clearStoredAuth,
  hasValidStoredAuth,
} from '../../shared/utils/auth'
import router from '../router'

export const useUserStore = defineStore('user', () => {
  const storedUser = getStoredUser()
  const hasValidAuth = hasValidStoredAuth()

  if (!hasValidAuth && storedUser) {
    clearStoredAuth()
  }

  const user = ref<User | null>(hasValidAuth ? storedUser : null)
  const userInfoLoaded = ref(false)

  const isLoggedIn = computed(() => !!user.value)
  const isAdmin = computed(() => user.value?.role === 'admin')

  function updateAuth(newUser: User) {
    user.value = newUser
    userInfoLoaded.value = true
    setStoredAuth(newUser)
  }

  function resetAuth() {
    user.value = null
    userInfoLoaded.value = false
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
    router.push(redirectPath)
    return logoutSuccess
  }

  async function fetchUserInfo(force = false) {
    if (userInfoLoaded.value && !force) return
    try {
      const res: any = await userApi.getInfo()
      if (res.code === 0 && res.data) {
        updateAuth(res.data.user)
      }
    } catch (e) {
      const code = Number((e as any)?.status ?? (e as any)?.code)
      if (code === 401 || code === 403) {
        resetAuth()
        return
      }
      userInfoLoaded.value = false
    }
  }

  async function updatePassword(oldPassword: string, newPassword: string) {
    const response = await fetch('/api/auth/change-password', {
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

  async function updateAvatar(avatar: string) {
    const nextAvatar = avatar.trim()
    if (!nextAvatar) {
      throw new Error('头像地址无效')
    }
    await userApi.updateProfile({ avatar: nextAvatar })
    if (!user.value) return
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
    updatePassword,
    updateAvatar,
  }
})
