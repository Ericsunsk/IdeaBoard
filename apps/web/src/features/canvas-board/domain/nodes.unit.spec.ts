import { describe, expect, it } from 'vitest'
import {
  requireImageNodeOutputCount,
  IMAGE_OUTPUT_COUNT_PRESET_OPTIONS,
  MAX_IMAGE_NODE_OUTPUT_COUNT,
  MIN_IMAGE_NODE_OUTPUT_COUNT,
} from './nodes'

describe('canvas node output count contract', () => {
  it('uses the canonical one-through-four preset range', () => {
    expect(MIN_IMAGE_NODE_OUTPUT_COUNT).toBe(1)
    expect(MAX_IMAGE_NODE_OUTPUT_COUNT).toBe(4)
    expect(IMAGE_OUTPUT_COUNT_PRESET_OPTIONS).toEqual([1, 2, 3, 4])
  })

  it('rejects values outside the requested output count contract', () => {
    expect(() => requireImageNodeOutputCount(99)).toThrow(
      'requested output count must be an integer between 1 and 4',
    )
  })
})
