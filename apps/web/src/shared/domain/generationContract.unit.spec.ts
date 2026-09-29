import { describe, expect, it } from 'vitest'
import {
  assertRequestedOutputCount,
  assertOutputResolution,
  GENERATION_PRIMITIVE,
  OUTPUT_RESOLUTION_OPTIONS,
  parseGenerationPrimitive,
  parseOutputResolution,
  parseRequestedOutputCount,
  REQUESTED_OUTPUT_COUNT_OPTIONS,
} from './generationContract'

describe('generation contract', () => {
  it('accepts only the two execution primitives', () => {
    expect(parseGenerationPrimitive(GENERATION_PRIMITIVE.textToImage)).toBe('text-to-image')
    expect(parseGenerationPrimitive(GENERATION_PRIMITIVE.imageToImage)).toBe('image-to-image')
    expect(parseGenerationPrimitive('generation')).toBeNull()
    expect(parseGenerationPrimitive('TEXT-TO-IMAGE')).toBeNull()
  })

  it('accepts only integer requested output counts from one through four', () => {
    expect(REQUESTED_OUTPUT_COUNT_OPTIONS).toEqual([1, 2, 3, 4])
    for (const count of REQUESTED_OUTPUT_COUNT_OPTIONS) {
      expect(parseRequestedOutputCount(count)).toBe(count)
      expect(assertRequestedOutputCount(count)).toBe(count)
    }
  })

  it('rejects invalid requested output counts without normalization', () => {
    for (const value of [0, 5, 8, 1.5, '2', Number.NaN]) {
      expect(parseRequestedOutputCount(value)).toBeNull()
      expect(() => assertRequestedOutputCount(value)).toThrow(
        'requested output count must be an integer between 1 and 4',
      )
    }
  })

  it('keeps the resolution catalog on exactly 1k, 2k, and 4k', () => {
    expect(OUTPUT_RESOLUTION_OPTIONS).toEqual(['1k', '2k', '4k'])
    for (const resolution of OUTPUT_RESOLUTION_OPTIONS) {
      expect(parseOutputResolution(resolution)).toBe(resolution)
      expect(assertOutputResolution(resolution)).toBe(resolution)
    }
  })

  it('canonicalizes API-compatible resolution spellings and rejects unowned values', () => {
    for (const [value, canonical] of [
      ['1K', '1k'],
      [' 2k ', '2k'],
      ['4K', '4k'],
    ] as const) {
      expect(parseOutputResolution(value)).toBe(canonical)
    }
    for (const value of ['default', 'DEFAULT', '', '  ', '8k', '1.5k', '1 k', null, undefined, 0]) {
      expect(parseOutputResolution(value)).toBeNull()
    }
    expect(() => assertOutputResolution('default')).toThrow(
      'output resolution must be one of 1k, 2k, 4k',
    )
  })
})
