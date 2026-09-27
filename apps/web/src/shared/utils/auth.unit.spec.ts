import { beforeEach, describe, expect, it } from 'vitest'
import {
  clearStoredAuth,
  getAuthStorageUserScope,
  getRecoverableAuthStorageUserScope,
  getStoredUser,
  handleAuthFailure,
  handleSilentAuthFailure,
  setStoredAuth,
} from './auth'
import type { User } from '../api/sessionUserType'

const adminUser: User = {
  public_id: '00000000-0000-0000-0000-000000000001',
  display_name: 'admin',
  storage_namespace: 'ADMIN1',
  avatar: '',
  role: 'admin',
  status: 1,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

beforeEach(() => {
  localStorage.clear()
  window.history.pushState({}, '', '/')
})

describe('handleAuthFailure', () => {
  it('keeps the last storage scope available for local draft recovery', () => {
    setStoredAuth(adminUser)

    expect(getAuthStorageUserScope()).toBe('public_00000000-0000-0000-0000-000000000001')

    clearStoredAuth()

    expect(getAuthStorageUserScope()).toBeNull()
    expect(getRecoverableAuthStorageUserScope()).toBe('public_00000000-0000-0000-0000-000000000001')
  })

  it('clears stored admin user state after an admin API 403', () => {
    setStoredAuth(adminUser)
    window.history.pushState({}, '', '/admin/monitor')

    const handled = handleAuthFailure({
      status: 403,
      requestUrl: '/api/admin/tasks',
    })

    expect(handled).toBe(true)
    expect(getStoredUser()).toBeNull()
    expect(window.location.pathname).toBe('/')
  })

  it('clears stored auth without redirecting after a session probe 401', () => {
    setStoredAuth(adminUser)
    window.history.pushState({}, '', '/admin/monitor')

    const handled = handleAuthFailure({
      status: 401,
      requestUrl: '/api/v1/user',
    })

    expect(handled).toBe(false)
    expect(getStoredUser()).toBeNull()
    expect(window.location.pathname).toBe('/admin/monitor')
  })
})

describe('handleSilentAuthFailure', () => {
  it('clears stored auth without navigating after a supplementary 401', () => {
    setStoredAuth(adminUser)
    window.history.pushState({}, '', '/user/boards')

    const handled = handleSilentAuthFailure({
      status: 401,
      requestUrl: '/api/v1/workspaces/ws_abc123/billing-quotes',
    })

    expect(handled).toBe(false)
    expect(getStoredUser()).toBeNull()
    expect(window.location.pathname).toBe('/user/boards')
  })

  it('leaves stored auth untouched when the policy ignores the failure', () => {
    setStoredAuth(adminUser)
    window.history.pushState({}, '', '/user/boards')

    const handled = handleSilentAuthFailure({
      status: 401,
      requestUrl: '/api/auth/sign-in/email',
    })

    expect(handled).toBe(false)
    expect(getStoredUser()).not.toBeNull()
    expect(window.location.pathname).toBe('/user/boards')
  })
})
