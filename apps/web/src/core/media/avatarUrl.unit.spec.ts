import { afterEach, describe, expect, it, vi } from 'vitest'
import { resolveMemberAvatar, resolveMemberInitial, resolveSizedAvatarUrl } from './avatarUrl'

const ROUTE = '/api/v1/public/users/AVTR01/avatar'

describe('resolveSizedAvatarUrl', () => {
  afterEach(() => vi.unstubAllGlobals())

  it.each([
    [88, 1, 128],
    [32, 1, 32],
    [36, 1, 64],
    [88, 2, 256],
    [32, 2, 64],
    [200, 1, 256],
  ] as Array<[number, number, number]>)('maps %ipx CSS at %idpr to %s', (width, dpr, size) => {
    expect(resolveSizedAvatarUrl(ROUTE, width, dpr)).toBe(`${ROUTE}?size=${size}`)
  })

  it('treats an invalid dpr as 1 and unknown widths as 32', () => {
    expect(resolveSizedAvatarUrl(ROUTE, 88, 0)).toBe(`${ROUTE}?size=128`)
    expect(resolveSizedAvatarUrl(ROUTE, 88, Number.NaN)).toBe(`${ROUTE}?size=128`)
    for (const value of [undefined, null, Number.NaN, 0, -40, 'wide']) {
      expect(resolveSizedAvatarUrl(ROUTE, value, 2)).toBe(`${ROUTE}?size=32`)
    }
  })

  it('defaults to the browser dpr and permits an explicit override', () => {
    vi.stubGlobal('devicePixelRatio', 2)
    expect(resolveSizedAvatarUrl(ROUTE, 88)).toBe(`${ROUTE}?size=256`)
    expect(resolveSizedAvatarUrl(ROUTE, 88, 1)).toBe(`${ROUTE}?size=128`)
  })

  it('defaults to 1 dpr without a browser', () => {
    vi.stubGlobal('window', undefined)
    expect(resolveSizedAvatarUrl(ROUTE, 88)).toBe(`${ROUTE}?size=128`)
  })

  it('passes provisioned external avatars through unsized', () => {
    expect(resolveSizedAvatarUrl('https://cdn.example/avatar.png', 88, 1)).toBe(
      'https://cdn.example/avatar.png',
    )
  })

  it('preserves the content version and replaces the requested size', () => {
    expect(resolveSizedAvatarUrl(`${ROUTE}?v=abcdef&size=32`, 88, 1)).toBe(
      `${ROUTE}?v=abcdef&size=128`,
    )
  })

  it('returns undefined for empty or non-string values', () => {
    for (const value of [undefined, null, '', '   ', 42]) {
      expect(resolveSizedAvatarUrl(value, 88, 1)).toBeUndefined()
    }
  })
})

describe('resolveMemberAvatar', () => {
  const ROUTE = '/api/v1/public/users/AVTR01/avatar?v=abc123'

  it('sizes the roster entry avatar first', () => {
    expect(resolveMemberAvatar({ userId: 'usr_a', avatar_url: ROUTE }, null)).toBe(
      `${ROUTE}&size=64`,
    )
  })

  it('falls back to the viewer session avatar for the viewer entry', () => {
    expect(
      resolveMemberAvatar({ userId: 'usr_me' }, { public_id: 'usr_me', avatar: ROUTE }),
    ).toBe(`${ROUTE}&size=64`)
  })

  it('prefers the entry avatar over the viewer avatar', () => {
    expect(
      resolveMemberAvatar(
        { userId: 'usr_me', avatar_url: '/avatar-entry.png' },
        { public_id: 'usr_me', avatar: ROUTE },
      ),
    ).toBe('/avatar-entry.png')
  })

  it('passes external URLs through unsized', () => {
    expect(
      resolveMemberAvatar({ userId: 'usr_a', avatar_url: 'https://cdn.example/a.png' }, null),
    ).toBe('https://cdn.example/a.png')
    expect(
      resolveMemberAvatar(
        { userId: 'usr_me' },
        { public_id: 'usr_me', avatar: 'https://cdn.example/me.png' },
      ),
    ).toBe('https://cdn.example/me.png')
  })

  it('returns undefined without an entry avatar or viewer match', () => {
    expect(resolveMemberAvatar({ userId: 'usr_a' }, null)).toBeUndefined()
    expect(
      resolveMemberAvatar({ userId: 'usr_a' }, { public_id: 'usr_me', avatar: ROUTE }),
    ).toBeUndefined()
    expect(
      resolveMemberAvatar({ userId: 'usr_me' }, { public_id: 'usr_me', avatar: '   ' }),
    ).toBeUndefined()
  })
})

describe('resolveMemberInitial', () => {
  it('takes the email first letter first', () => {
    expect(resolveMemberInitial({ userId: 'usr_a', email: 'a@example.com' })).toBe('A')
    expect(resolveMemberInitial({ userId: 'other', email: 'boss@example.com' })).toBe('B')
  })

  it('strips the usr_ prefix from the userId fallback', () => {
    expect(resolveMemberInitial({ userId: 'usr_a' })).toBe('A')
    expect(resolveMemberInitial({ userId: 'other' })).toBe('O')
  })

  it('falls back to the userId when the email is empty', () => {
    expect(resolveMemberInitial({ userId: 'usr_a', email: '' })).toBe('A')
    expect(resolveMemberInitial({ userId: 'usr_a', email: null })).toBe('A')
  })

  it('returns U without an email or userId initial', () => {
    expect(resolveMemberInitial({ userId: '' })).toBe('U')
    expect(resolveMemberInitial({ userId: 'usr_' })).toBe('U')
  })
})
