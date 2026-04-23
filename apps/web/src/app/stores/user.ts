import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { User } from '../../shared/types'
import { userApi, authApi } from '../../shared/api'
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
    const res: any = await authApi.login({ username, password })
    if (res.code === 0) {
      const loginUser = res.data?.user as User | undefined
      if (loginUser) {
        updateAuth(loginUser)
      } else {
        await fetchUserInfo(true)
      }
      if (!user.value) {
        throw new Error('登录状态恢复失败')
      }
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
    let logoutSuccess = true
    try {
      await userApi.logout()
    } catch (e) {
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
    const res: any = await userApi.updatePassword({
      old_password: oldPassword,
      new_password: newPassword
    })
    if (res.code !== 0) {
      throw new Error(res.msg)
    }
    resetAuth()
    return true
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
    updatePassword
  }
})
