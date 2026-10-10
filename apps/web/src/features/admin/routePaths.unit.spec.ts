import { describe, expect, it } from 'vitest'
import { ADMIN_LOGIN_PATH, ADMIN_ROOT_PATH, isPathUnderRoute } from './routePaths'
import { USER_LOGIN_PATH } from '../workspace/routePaths'

describe('isPathUnderRoute', () => {
  it('matches the route root itself', () => {
    expect(isPathUnderRoute('/admin', ADMIN_ROOT_PATH)).toBe(true)
  })

  it('matches child paths under the route root', () => {
    expect(isPathUnderRoute('/admin/users', ADMIN_ROOT_PATH)).toBe(true)
    expect(isPathUnderRoute('/admin/users/42', ADMIN_ROOT_PATH)).toBe(true)
  })

  it('ignores query strings and hashes when matching', () => {
    expect(isPathUnderRoute('/admin?tab=overview', ADMIN_ROOT_PATH)).toBe(true)
    expect(isPathUnderRoute('/admin/users?page=1#top', ADMIN_ROOT_PATH)).toBe(true)
  })

  it('rejects prefix-spoofed paths', () => {
    expect(isPathUnderRoute('/adminish', ADMIN_ROOT_PATH)).toBe(false)
    expect(isPathUnderRoute('/admin-login', ADMIN_ROOT_PATH)).toBe(false)
  })

  it('matches auth login child paths without accepting prefix spoofs', () => {
    expect(isPathUnderRoute('/user/login/callback', USER_LOGIN_PATH)).toBe(true)
    expect(isPathUnderRoute('/admin/login/session', ADMIN_LOGIN_PATH)).toBe(true)
    expect(isPathUnderRoute('/user/loginish', USER_LOGIN_PATH)).toBe(false)
  })
})
