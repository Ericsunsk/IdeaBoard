import { describe, expect, it } from 'vitest'
import { isSafeInternalRedirectPath } from './auth'

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
