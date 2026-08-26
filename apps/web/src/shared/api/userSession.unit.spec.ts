import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchCurrentUserInfo } from './userSession'

const mocks = vi.hoisted(() => ({
  getUserInfo: vi.fn(),
}))

vi.mock('./generated-query/user/user', () => ({
  getUserInfo: mocks.getUserInfo,
}))

describe('user session API boundary', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns the session data without the transport envelope', async () => {
    const user = {
      public_id: '00000000-0000-4000-8000-000000000007',
      display_name: 'Alice',
      storage_namespace: 'USER0007',
      avatar: '',
      role: 'user',
      status: 1,
      created_at: '2026-01-01T00:00:00.000Z',
      updated_at: '2026-01-01T00:00:00.000Z',
    }
    mocks.getUserInfo.mockResolvedValue({ code: 0, msg: 'ok', data: { user } })

    await expect(fetchCurrentUserInfo()).resolves.toEqual({ user })
  })

  it('rejects a successful HTTP response with a failed business code', async () => {
    mocks.getUserInfo.mockResolvedValue({ code: 503, msg: 'dial tcp 127.0.0.1:5432' })

    await expect(fetchCurrentUserInfo()).rejects.toThrow('Could not load your session. Please sign in again.')
  })

  it('rejects a malformed successful response without a user', async () => {
    mocks.getUserInfo.mockResolvedValue({ code: 0, msg: 'ok', data: {} })

    await expect(fetchCurrentUserInfo()).rejects.toThrow('Could not load your session. Please sign in again.')
  })
})
