import { describe, expect, it } from 'vitest'
import {
  createModelSuggestions,
  mergeDetectedModelValues,
  normalizeDetectedModelValues,
} from './providerModelOptions'

describe('provider model options', () => {
  it('merges detected models without blank or case duplicate values', () => {
    expect(mergeDetectedModelValues([' GPT-4o ', 'gemini'], ['gpt-4o', '', ' Imagen '])).toEqual([
      'GPT-4o',
      'gemini',
      'Imagen',
    ])
  })

  it('filters suggestions across detected and selected models', () => {
    expect(
      createModelSuggestions({
        detectedValues: ['gemini-2.5-flash', 'imagen-4'],
        selectedValues: ['gpt-4o'],
        query: '4',
      }),
    ).toEqual(['imagen-4', 'gpt-4o'])
  })

  it('caps normalized detected models', () => {
    expect(normalizeDetectedModelValues(['a', 'b', 'c'], 2)).toEqual(['a', 'b'])
  })
})
