import { describe, expect, it } from 'vitest'
import { stripUndefined } from './stripUndefined'

describe('stripUndefined', () => {
  it('drops undefined-valued keys and keeps the rest by reference value', () => {
    expect(stripUndefined({ a: 1, b: undefined, c: 'x' })).toEqual({ a: 1, c: 'x' })
  })

  it('keeps null, false, zero, and empty strings', () => {
    expect(stripUndefined({ a: null, b: false, c: 0, d: '' })).toEqual({
      a: null,
      b: false,
      c: 0,
      d: '',
    })
  })

  it('returns an empty object when every value is undefined', () => {
    expect(stripUndefined({ a: undefined })).toEqual({})
  })

  it('does not mutate the input', () => {
    const input = { a: 1, b: undefined as number | undefined }
    stripUndefined(input)
    expect(input).toEqual({ a: 1, b: undefined })
    expect('b' in input).toBe(true)
  })
})
