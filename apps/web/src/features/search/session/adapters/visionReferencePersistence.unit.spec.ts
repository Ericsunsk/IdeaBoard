import { beforeEach, describe, expect, it } from 'vitest'
import { getCachedVisionReferences, setCachedVisionReferences } from './visionReferencePersistence'

const externalReference = {
  provider: 'unsplash',
  id: 'photo-1',
  preview_url: 'https://images.unsplash.com/photo-1',
}

describe('Vision reference storage adapter', () => {
  beforeEach(() => sessionStorage.clear())

  it('round-trips the reference projection without interpreting presentation expiry', () => {
    setCachedVisionReferences([
      {
        key: 'unsplash:photo-1',
        externalReference,
        previewUrl: '/api/media/previews/token/thumb',
        previewExpiresAt: new Date(Date.now() - 1).toISOString(),
      },
    ])
    const [reference] = getCachedVisionReferences()
    expect(reference?.externalReference).toEqual(externalReference)
    expect(reference?.previewUrl).toBe('/api/media/previews/token/thumb')
  })

  it('leaves presentation policy to the reference collection', () => {
    const previewExpiresAt = new Date(Date.now() + 86_400_000).toISOString()
    setCachedVisionReferences([
      {
        key: 'unsplash:photo-1',
        externalReference,
        previewUrl: '/api/media/previews/token/thumb',
        previewExpiresAt,
      },
    ])
    expect(getCachedVisionReferences()[0]?.previewUrl).toBe('/api/media/previews/token/thumb')
    setCachedVisionReferences([
      {
        key: 'unsplash:photo-1',
        externalReference,
        previewUrl: externalReference.preview_url,
        previewExpiresAt,
      },
    ])
    expect(getCachedVisionReferences()[0]?.previewUrl).toBe(externalReference.preview_url)
  })

  it('ignores malformed persisted shapes', () => {
    sessionStorage.setItem(
      'ideaboard:search:vision_refs:anonymous',
      JSON.stringify([
        null,
        { key: 'missing-preview', libraryAssetID: 'lasset_1' },
        { key: 'invalid-external', previewUrl: '', externalReference: { id: 3 } },
        { key: 'valid', previewUrl: '', externalReference, file: { name: 'invalid' } },
      ]),
    )
    expect(getCachedVisionReferences()).toEqual([
      { key: 'valid', previewUrl: '', externalReference },
    ])
  })
})
