import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { User } from '../types'
import { userApi, authApi } from '../api'
import {
  getStoredToken,
  getStoredUser,
  setStoredAuth,
  clearStoredAuth,
  hasValidStoredAuth,
} from '../utils/auth'
import router from '../router'

export const useUserStore = defineStore('user', () => {
  const storedToken = getStoredToken()
  const storedUser = getStoredUser()
  const hasValidAuth = hasValidStoredAuth()

  if (!hasValidAuth && (storedToken || storedUser)) {
    clearStoredAuth()
  }

  const token = ref<string | null>(hasValidAuth ? storedToken : null)
  const user = ref<User | null>(hasValidAuth ? storedUser : null)
  const userInfoLoaded = ref(hasValidAuth)

  const isLoggedIn = computed(() => !!(token.value && user.value))
  const isAdmin = computed(() => user.value?.role === 'admin')

  function updateAuth(newToken: string, newUser: User) {
    token.value = newToken
    user.value = newUser
    userInfoLoaded.value = true
    setStoredAuth(newToken, newUser)
  }

  function resetAuth() {
    token.value = null
    user.value = null
    userInfoLoaded.value = false
    clearStoredAuth()
  }

  async function login(username: string, password: string) {
    const res: any = await authApi.login({ username, password })
    if (res.code === 0 && res.data) {
      updateAuth(res.data.token, res.data.user)
      return true
    }
    throw new Error(res.msg)
  }

  async function register(username: string, password: string) {
    const res: any = await authApi.register({ username, password })
    if (res.code === 0) {
      return true
    }
    throw new Error(res.msg)
  }

  async function logout(redirectPath = '/user/login') {
    try {
      await userApi.logout()
    } catch (e) {
    }
    resetAuth()
    router.push(redirectPath)
  }

  async function fetchUserInfo(force = false) {
    if (!token.value) return
    if (userInfoLoaded.value && !force) return
    try {
      const res: any = await userApi.getInfo()
      if (res.code === 0 && res.data) {
        user.value = res.data.user
        userInfoLoaded.value = true
        setStoredAuth(token.value!, res.data.user)
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
    const res: any = await userApi.updatePassword({
      old_password: oldPassword,
      new_password: newPassword
    })
    if (res.code !== 0) {
      throw new Error(res.msg)
    }
    return true
  }

  return {
    token,
    user,
    isLoggedIn,
    isAdmin,
    setAuth: updateAuth,
    clearAuth: resetAuth,
    login,
    register,
    logout,
    fetchUserInfo,
    updatePassword
  }
})
