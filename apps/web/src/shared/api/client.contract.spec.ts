// Request-level 401 policy contract (frontend 401-policy slice).
//
// MSW-level: exercises the real `api.request` transport (ofetch) without
// mocking generated-query modules. Default requests preserve the global
// redirect-login behavior; `authFailure: 'silent'` returns the error for
// local handling after clearing local auth state — never navigating.
import { beforeEach, describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../test/msw'
import { api } from './client'
import { getStoredUser, setStoredAuth } from '../utils/auth'
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
})
