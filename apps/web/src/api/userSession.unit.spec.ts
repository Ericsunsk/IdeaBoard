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

  it('maps the canonical principal with the wire id renamed to public_id', async () => {
    mocks.getUser.mockResolvedValue({
      data: {
        id: 'usr_000000000000000000000007',
        display_name: 'Alice',
        role: 'user',
        personal_workspace_id: 'ws_000000000000000000000001',
      },
    })

    await expect(fetchCurrentUserInfo()).resolves.toEqual({
      user: {
        public_id: 'usr_000000000000000000000007',
        display_name: 'Alice',
        role: 'user',
        personal_workspace_id: 'ws_000000000000000000000001',
      },
    })
  })

  it('ignores a legacy public_id key: only the canonical wire id converts', () => {
    // No caller sends `public_id` on this surface; the old fallback is gone.
    expect(toSessionUser({ public_id: 'usr_legacy' })).toBeNull()
    expect(toSessionUser({ id: 'usr_1', public_id: 'usr_legacy', display_name: 'Alice' })).toEqual({
      public_id: 'usr_1',
      display_name: 'Alice',
    })
  })

  it('projects the durable avatar and profile facts while dropping unrelated wire fields', () => {
    // avatar is the authoritative profile projection; storage namespace,
    // status, and timestamps ride the same /user boundary. Email stays
    // owned by Better Auth and never lands on this view.
    expect(
      toSessionUser({
        id: 'usr_1',
        display_name: 'Alice',
        role: 'user',
        avatar: 'https://cdn.example/a.png',
        email: 'alice@example.test',
        storage_namespace: 'USER0001',
        status: 1,
        created_at: '2026-01-01T00:00:00.000Z',
        updated_at: '2026-01-02T00:00:00.000Z',
      }),
    ).toEqual({
      public_id: 'usr_1',
      display_name: 'Alice',
      role: 'user',
      avatar: 'https://cdn.example/a.png',
      storage_namespace: 'USER0001',
      status: 1,
      created_at: '2026-01-01T00:00:00.000Z',
      updated_at: '2026-01-02T00:00:00.000Z',
    })
  })

  it('ignores a non-numeric status instead of projecting it', () => {
    expect(toSessionUser({ id: 'usr_1', status: 'banned' })).toEqual({ public_id: 'usr_1' })
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
