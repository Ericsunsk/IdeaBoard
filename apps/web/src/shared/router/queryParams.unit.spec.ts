import { describe, expect, it, vi } from 'vitest'
import { stringParam, stringParamWhere } from './queryParams'

describe('stringParam', () => {
  it('passes single string values through, including empty strings', () => {
    expect(stringParam('boards')).toBe('boards')
    expect(stringParam('')).toBe('')
  })

  it('maps null and missing params to undefined', () => {
    expect(stringParam(null)).toBeUndefined()
    expect(stringParam(undefined)).toBeUndefined()
  })

  it('collapses repeated params to their first string entry', () => {
    expect(stringParam(['a', 'b'])).toBe('a')
    expect(stringParam([''])).toBe('')
    expect(stringParam([null])).toBeUndefined()
    expect(stringParam([])).toBeUndefined()
  })

  it('maps non-string scalars to undefined', () => {
    expect(stringParam(42 as unknown as string)).toBeUndefined()
  })
})

describe('stringParamWhere', () => {
  it('returns the value only when the predicate passes', () => {
    expect(stringParamWhere('1', (value) => value === '1')).toBe('1')
    expect(stringParamWhere('0', (value) => value === '1')).toBeUndefined()
  })

  it('applies the predicate to the first entry of repeated params', () => {
    expect(stringParamWhere(['1', '2'], (value) => value === '1')).toBe('1')
    expect(stringParamWhere(['0', '1'], (value) => value === '1')).toBeUndefined()
  })

  it('never calls the predicate for absent or non-string values', () => {
    const validate = vi.fn(() => true)
    expect(stringParamWhere(null, validate)).toBeUndefined()
    expect(stringParamWhere(undefined, validate)).toBeUndefined()
    expect(stringParamWhere([null], validate)).toBeUndefined()
    expect(validate).not.toHaveBeenCalled()
  })
})
