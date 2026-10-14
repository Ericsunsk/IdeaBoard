import { afterEach, describe, expect, it, vi } from 'vitest'
import { resolveSizedAvatarUrl } from './avatarUrl'

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
