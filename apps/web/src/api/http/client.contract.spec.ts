// Request-level 401 policy contract (frontend 401-policy slice).
//
// MSW-level: exercises the real `api.request` transport (ofetch) without
// mocking generated-query modules. Default requests preserve the global
// redirect-login behavior; `authFailure: 'silent'` returns the error for
// local handling after clearing local auth state — never navigating.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../test/msw'
import { api } from './client'
import { getStoredUser, setStoredAuth } from '../../core/utils/auth'
import type { User } from './sessionUserType'

const storedUser: User = {
  public_id: '00000000-0000-0000-0000-000000000001',
  display_name: 'policy-user',
  avatar: '',
  role: 'user',
}

function unauthorized() {
  return HttpResponse.json({ code: 401, msg: '' }, { status: 401 })
}

beforeEach(() => {
  localStorage.clear()
  window.history.pushState({}, '', '/user/boards')
})

afterEach(() => vi.restoreAllMocks())

describe('api.request auth failure policy', () => {
  it('redirects to login by default on a supplementary 401', async () => {
    setStoredAuth(storedUser)
    useMswHandlers(http.get('/api/supplementary-hint', () => unauthorized()))

    await expect(api.request('/supplementary-hint', { method: 'GET' })).rejects.toThrow()

    expect(getStoredUser()).toBeNull()
    expect(window.location.pathname).toBe('/user/login')
  })

  it('returns the 401 error without navigating when silent', async () => {
    setStoredAuth(storedUser)
    useMswHandlers(http.get('/api/supplementary-hint', () => unauthorized()))

    await expect(
      api.request('/supplementary-hint', { method: 'GET', authFailure: 'silent' }),
    ).rejects.toThrow()

    expect(getStoredUser()).toBeNull()
    expect(window.location.pathname).toBe('/user/boards')
  })

  it('redirects exactly once for concurrent and repeated 401 responses until a new identity is stored', async () => {
    setStoredAuth(storedUser)
    window.history.replaceState({}, '', '/user/boards?view=recent#owned')
    const navigate = vi.spyOn(window.location, 'href', 'set')
    useMswHandlers(http.get('/api/supplementary-hint', () => unauthorized()))

    const results = await Promise.allSettled([
      api.request('/supplementary-hint', { method: 'GET' }),
      api.request('/supplementary-hint', { method: 'GET' }),
    ])
    expect(results.map((result) => result.status)).toEqual(['rejected', 'rejected'])
    await expect(api.request('/supplementary-hint', { method: 'GET' })).rejects.toThrow()

    expect(getStoredUser()).toBeNull()
    expect(navigate).toHaveBeenCalledTimes(1)
    expect(navigate).toHaveBeenCalledWith(
      '/user/login?redirect=%2Fuser%2Fboards%3Fview%3Drecent%23owned',
    )
    expect(window.location.pathname).toBe('/user/login')

    setStoredAuth({ ...storedUser, public_id: '00000000-0000-0000-0000-000000000002' })
    window.history.replaceState({}, '', '/user/boards')
    await expect(api.request('/supplementary-hint', { method: 'GET' })).rejects.toThrow()
    expect(navigate).toHaveBeenCalledTimes(2)
  })
})
