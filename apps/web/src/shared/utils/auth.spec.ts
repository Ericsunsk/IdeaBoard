import { beforeEach, describe, expect, it } from 'vitest'
import {
  clearStoredAuth,
  getStoredUser,
  handleAuthFailure,
  isSafeInternalRedirectPath,
  setStoredAuth,
} from './auth'
import type { User } from '../types'

const adminUser: User = {
  id: 1,
  uid: 'ADMIN1',
  username: 'admin',
  auth_user_id: 'auth-admin',
  role: 'admin',
  status: 1,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

beforeEach(() => {
  clearStoredAuth()
  window.history.pushState({}, '', '/user/home')
})

describe('isSafeInternalRedirectPath', () => {
  it('allows safe internal redirect paths with query and hash', () => {
    expect(isSafeInternalRedirectPath('/share?token=abc')).toBe(true)
    expect(isSafeInternalRedirectPath('/share?token=abc#join')).toBe(true)
    expect(isSafeInternalRedirectPath('/user/boards?tab=recent#top')).toBe(true)
    expect(isSafeInternalRedirectPath('/admin/users?page=1')).toBe(true)
  })

  it('rejects external, protocol-relative, and backslash redirects', () => {
    expect(isSafeInternalRedirectPath('https://example.com/share?token=abc')).toBe(false)
    expect(isSafeInternalRedirectPath('javascript:alert(1)')).toBe(false)
    expect(isSafeInternalRedirectPath('//example.com/share?token=abc')).toBe(false)
    expect(isSafeInternalRedirectPath('/share\\evil')).toBe(false)
    expect(isSafeInternalRedirectPath('/share%5Cevil')).toBe(false)
  })

  it('rejects auth-loop and prefix-spoofed redirects', () => {
    expect(isSafeInternalRedirectPath('/user/login?redirect=/share?token=abc')).toBe(false)
    expect(isSafeInternalRedirectPath('/user/register#start')).toBe(false)
    expect(isSafeInternalRedirectPath('/admin/login?redirect=/admin/users')).toBe(false)
    expect(isSafeInternalRedirectPath('/shareevil?token=abc')).toBe(false)
  })
})

describe('handleAuthFailure', () => {
  it('clears stored admin user state after an admin API 403', () => {
    setStoredAuth(adminUser)
    window.history.pushState({}, '', '/admin/task-monitor')

    const handled = handleAuthFailure({
      status: 403,
      requestUrl: '/api/admin/tasks',
    })

    expect(handled).toBe(true)
    expect(getStoredUser()).toBeNull()
    expect(window.location.pathname).toBe('/user/home')
  })
})
