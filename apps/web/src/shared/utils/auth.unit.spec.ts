import { beforeEach, describe, expect, it } from 'vitest'
import {
  clearStoredAuth,
  getAuthStorageUserScope,
  getRecoverableAuthStorageUserScope,
  getStoredUser,
  handleAuthFailure,
  setStoredAuth,
} from './auth'
import type { User } from '../api/generated-query/model'

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
    expect(getRecoverableAuthStorageUserScope()).toBe(
      'public_00000000-0000-0000-0000-000000000001',
    )
  })

  it('clears stored admin user state after an admin API 403', () => {
    setStoredAuth(adminUser)
    window.history.pushState({}, '', '/admin/task-monitor')

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
    window.history.pushState({}, '', '/admin/task-monitor')

    const handled = handleAuthFailure({
      status: 401,
      requestUrl: '/api/user/info',
    })

    expect(handled).toBe(false)
    expect(getStoredUser()).toBeNull()
    expect(window.location.pathname).toBe('/admin/task-monitor')
  })
})
