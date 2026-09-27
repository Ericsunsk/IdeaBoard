import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchCurrentUserInfo, toSessionUser } from './userSession'

const mocks = vi.hoisted(() => ({
  getUser: vi.fn(),
}))

vi.mock('./generated-query/public/user/user', () => ({
  getUser: mocks.getUser,
}))

describe('user session API boundary', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns the session data without the transport envelope', async () => {
    const me = {
      id: 'usr_000000000000000000000007',
      public_id: 'usr_000000000000000000000007',
      display_name: 'Alice',
      avatar: '',
      role: 'user',
    }
    mocks.getUser.mockResolvedValue({ data: me })

    await expect(fetchCurrentUserInfo()).resolves.toEqual({
      user: {
        public_id: 'usr_000000000000000000000007',
        display_name: 'Alice',
        role: 'user',
      },
    })
  })

  it('rejects when the principal request fails', async () => {
    mocks.getUser.mockRejectedValue(
      Object.assign(new Error('Could not load your session. Please sign in again.'), {
        status: 503,
      }),
    )

    await expect(fetchCurrentUserInfo()).rejects.toThrow(
      'Could not load your session. Please sign in again.',
    )
  })

  it('rejects a malformed successful response without a principal', async () => {
    mocks.getUser.mockResolvedValue({ data: {} })

    await expect(fetchCurrentUserInfo()).rejects.toThrow(
      'Could not load your session. Please sign in again.',
    )
  })

  it('drops unknown principal fields to undefined instead of leaking them', () => {
    expect(toSessionUser(null)).toBeNull()
    expect(toSessionUser({})).toBeNull()
    expect(toSessionUser({ data: undefined })).toBeNull()
  })
})
