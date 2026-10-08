import { describe, expect, it } from 'vitest'
import {
  EXTERNAL_IMAGE_PROVIDERS,
  EXTERNAL_IMAGE_PROVIDER_LABELS,
  isExternalImageProvider,
} from './externalImageProviders'

describe('external image providers', () => {
  it('keeps the supported provider order and display names', () => {
    expect(EXTERNAL_IMAGE_PROVIDERS).toEqual(['unsplash', 'pexels'])
    expect(
      EXTERNAL_IMAGE_PROVIDERS.map((provider) => EXTERNAL_IMAGE_PROVIDER_LABELS[provider]),
    ).toEqual(['Unsplash', 'Pexels'])
  })

  it('recognizes only exact supported IDs', () => {
    for (const provider of EXTERNAL_IMAGE_PROVIDERS)
      expect(isExternalImageProvider(provider)).toBe(true)
    for (const value of [
      'other',
      'Unsplash',
      ' unsplash',
      '',
      null,
      undefined,
      {},
      ['pexels'],
      1,
    ]) {
      expect(isExternalImageProvider(value)).toBe(false)
    }
  })
})
