import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { User } from '../types'
import { userApi, authApi } from '../api'
import router from '../router'

function clearStoredAuth() {
  localStorage.removeItem('token')
  localStorage.removeItem('user')
}

function readStoredUser(): User | null {
  const raw = localStorage.getItem('user')
  if (!raw) {
    return null
  }

  try {
    return JSON.parse(raw) as User
  } catch {
    clearStoredAuth()
    return null
  }
}

export const useUserStore = defineStore('user', () => {
  const storedToken = localStorage.getItem('token')
  const storedUser = readStoredUser()
  const hasValidAuth = !!(storedToken && storedUser)

  if (!hasValidAuth && (storedToken || storedUser)) {
    clearStoredAuth()
  }

  const token = ref<string | null>(hasValidAuth ? storedToken : null)
  const user = ref<User | null>(hasValidAuth ? storedUser : null)

  const isLoggedIn = computed(() => !!(token.value && user.value))
  const isAdmin = computed(() => user.value?.role === 'admin')

  function setAuth(newToken: string, newUser: User) {
    token.value = newToken
    user.value = newUser
    localStorage.setItem('token', newToken)
    localStorage.setItem('user', JSON.stringify(newUser))
  }

  function clearAuth() {
    token.value = null
    user.value = null
    clearStoredAuth()
  }

  async function login(username: string, password: string) {
    const res: any = await authApi.login({ username, password })
    if (res.code === 0 && res.data) {
      setAuth(res.data.token, res.data.user)
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
    clearAuth()
    router.push(redirectPath)
  }

  async function fetchUserInfo() {
    if (!token.value) return
    try {
      const res: any = await userApi.getInfo()
      if (res.code === 0 && res.data) {
        user.value = res.data.user
        localStorage.setItem('user', JSON.stringify(res.data.user))
      }
    } catch (e) {
      clearAuth()
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
    setAuth,
    clearAuth,
    login,
    register,
    logout,
    fetchUserInfo,
    updatePassword
  }
})
