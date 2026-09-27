import { describe, expect, it } from 'vitest'
import {
  isSafeInternalRedirectPath,
  normalizeSafeInternalRedirectPath,
  resolveAuthFailureAction,
} from './sessionPolicy'

describe('redirect safety policy', () => {
  it('allows safe internal redirect paths with query and hash', () => {
    expect(isSafeInternalRedirectPath('/share?token=abc')).toBe(true)
    expect(isSafeInternalRedirectPath('/share?token=abc#join')).toBe(true)
    expect(isSafeInternalRedirectPath('/user/boards?tab=recent#top')).toBe(true)
    expect(isSafeInternalRedirectPath('/admin/users?page=1#x')).toBe(true)
  })

  it('rejects external, protocol-relative, and backslash redirects', () => {
    expect(isSafeInternalRedirectPath('https://example.com/share?token=abc')).toBe(false)
    expect(isSafeInternalRedirectPath('javascript:alert(1)')).toBe(false)
    expect(isSafeInternalRedirectPath('//example.com/share?token=abc')).toBe(false)
    expect(isSafeInternalRedirectPath('/share\\evil')).toBe(false)
    expect(isSafeInternalRedirectPath('/share%5Cevil')).toBe(false)
    expect(isSafeInternalRedirectPath('/admin/%5Cevil')).toBe(false)
  })

  it('rejects auth-loop and prefix-spoofed redirects', () => {
    expect(isSafeInternalRedirectPath('/user/login?redirect=/share?token=abc')).toBe(false)
    expect(isSafeInternalRedirectPath('/user/register#start')).toBe(false)
    expect(isSafeInternalRedirectPath('/admin/login?redirect=/admin/users')).toBe(false)
    expect(isSafeInternalRedirectPath('/adminish/users')).toBe(false)
    expect(isSafeInternalRedirectPath('/shareevil?token=abc')).toBe(false)
  })

  it('rejects paths that would change route scope after URL normalization', () => {
    expect(isSafeInternalRedirectPath('/admin/../user/boards')).toBe(false)
    expect(isSafeInternalRedirectPath('/admin/%2e%2e/user/boards')).toBe(false)
    expect(isSafeInternalRedirectPath('/admin/./users')).toBe(false)
  })

  it('returns a normalized pathname and fullPath for safe internal redirects', () => {
    expect(normalizeSafeInternalRedirectPath('/admin/users?page=1#x')).toEqual({
      pathname: '/admin/users',
      fullPath: '/admin/users?page=1#x',
    })
  })
})

describe('resolveAuthFailureAction', () => {
  it('redirects admin users home after an admin API 403 on an admin page', () => {
    expect(
      resolveAuthFailureAction({
        status: 403,
        requestUrl: '/api/admin/tasks',
        currentPathname: '/admin/monitor',
        currentFullPath: '/admin/monitor',
      }),
    ).toEqual({ kind: 'redirect-home', targetPath: '/' })
  })

  it('redirects admin users home after a control API 403 on an admin page', () => {
    expect(
      resolveAuthFailureAction({
        status: 403,
        requestUrl: '/api/control/v1/platform/statistics?range=7d',
        currentPathname: '/admin/statistics',
        currentFullPath: '/admin/statistics',
      }),
    ).toEqual({ kind: 'redirect-home', targetPath: '/' })
  })

  it('clears auth without redirecting for session probe 401', () => {
    expect(
      resolveAuthFailureAction({
        status: 401,
        requestUrl: '/api/v1/user',
        currentPathname: '/admin/monitor',
        currentFullPath: '/admin/monitor',
      }),
    ).toEqual({ kind: 'clear-only' })
  })

  it('clears auth without redirecting for a boot-time probe 401 on the login page', () => {
    expect(
      resolveAuthFailureAction({
        status: 401,
        requestUrl: 'http://localhost:6000/api/v1/user',
        currentPathname: '/user/login',
        currentFullPath: '/user/login?redirect=%2Fuser%2Fboards',
      }),
    ).toEqual({ kind: 'clear-only' })
  })

  it('keeps redirect-login for session sub-resource 401', () => {
    expect(
      resolveAuthFailureAction({
        status: 401,
        requestUrl: 'http://localhost:6000/api/v1/user/avatar',
        currentPathname: '/user/boards',
        currentFullPath: '/user/boards',
      }),
    ).toEqual({
      kind: 'redirect-login',
      targetPath: '/user/login?redirect=%2Fuser%2Fboards',
    })
  })

  it('ignores auth endpoint 401 responses', () => {
    expect(
      resolveAuthFailureAction({
        status: 401,
        requestUrl: '/api/auth/sign-in/email',
        currentPathname: '/user/login',
        currentFullPath: '/user/login',
      }),
    ).toEqual({ kind: 'ignore' })
  })

  it('preserves the current user destination after a protected session is revoked', () => {
    expect(
      resolveAuthFailureAction({
        status: 401,
        requestUrl: '/api/boards/board-1',
        currentPathname: '/user/canvas',
        currentFullPath: '/user/canvas?boardId=board-1',
      }),
    ).toEqual({
      kind: 'redirect-login',
      targetPath: '/user/login?redirect=%2Fuser%2Fcanvas%3FboardId%3Dboard-1',
    })
  })

  it('preserves the current Admin destination after an admin session is revoked', () => {
    expect(
      resolveAuthFailureAction({
        payload: { code: 401 },
        requestUrl: '/api/admin/users',
        currentPathname: '/admin/users',
        currentFullPath: '/admin/users?page=1',
      }),
    ).toEqual({
      kind: 'redirect-login',
      targetPath: '/admin/login?redirect=%2Fadmin%2Fusers%3Fpage%3D1',
    })
  })

  it('preserves a share join destination across login', () => {
    expect(
      resolveAuthFailureAction({
        status: 401,
        requestUrl: '/api/boards/share-token',
        currentPathname: '/share',
        currentFullPath: '/share?token=abc',
      }),
    ).toEqual({
      kind: 'redirect-login',
      targetPath: '/user/login?redirect=%2Fshare%3Ftoken%3Dabc',
    })
  })

  it('clears auth without redirecting when no browser route context is available', () => {
    expect(resolveAuthFailureAction({ status: 401, requestUrl: '/api/boards' })).toEqual({
      kind: 'clear-only',
    })
  })
})
