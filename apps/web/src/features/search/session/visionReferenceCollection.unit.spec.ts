import { describe, expect, it, vi } from 'vitest'
import { mediaDisplayFixture } from '../../../test/mediaDisplay'
import type { SearchResult } from '../core/models'
import { createVisionReferenceCollection, MAX_REFERENCE_COUNT } from './visionReferenceCollection'
import {
  VISION_REFERENCE_MAX_BYTES,
  VISION_REFERENCE_MAX_COUNT,
} from '../../../core/media/visionIngressPolicy'
import { MAX_REFERENCE_BYTES } from '../core/visionReferencePolicy'

function reference(key: string, file?: File) {
  return { key, previewUrl: `https://example.com/${key}.jpg`, file }
}

describe('visionReferenceCollection', () => {
  it('sources its budgets from the shared vision ingress policy (no drift)', () => {
    expect(MAX_REFERENCE_COUNT).toBe(VISION_REFERENCE_MAX_COUNT)
    expect(MAX_REFERENCE_BYTES).toBe(VISION_REFERENCE_MAX_BYTES)
  })

  it('deduplicates references and tracks slot count and capacity', () => {
    const collection = createVisionReferenceCollection()
    expect(collection.add(reference('a'))).toBe(true)
    expect(collection.add(reference('a'))).toBe(false)
    expect(collection.slotCount.value).toBe(1)
    expect(collection.has('a')).toBe(true)
    expect(collection.has('b')).toBe(false)
    expect(collection.canAdd.value).toBe(true)
  })

  it('calculates currentBytes from local file references', () => {
    const collection = createVisionReferenceCollection()
    expect(collection.currentBytes()).toBe(0)
    collection.add(reference('local', new File(['hello-bytes'], 'test.png', { type: 'image/png' })))
    expect(collection.currentBytes()).toBe(11)
  })

  it('removes local object URLs on remove and clear', () => {
    const revoke = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined)
    const collection = createVisionReferenceCollection([
      reference('local', new File(['image'], 'local.jpg', { type: 'image/jpeg' })),
    ])
    collection.references.value[0]!.previewUrl = 'blob:local'

    expect(collection.remove('local')).toBe(true)
    expect(revoke).toHaveBeenCalledWith('blob:local')
    expect(collection.references.value).toHaveLength(0)
    revoke.mockRestore()
  })

  it('normalizes initial references by key, capacity, and local preview ownership', () => {
    const revoke = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined)
    const collection = createVisionReferenceCollection([
      reference('keep', new File(['image'], 'keep.jpg', { type: 'image/jpeg' })),
      { key: 'keep', previewUrl: 'blob:duplicate' },
      ...Array.from({ length: MAX_REFERENCE_COUNT }, (_, index) => ({
        key: `extra-${index}`,
        previewUrl: `blob:extra-${index}`,
      })),
    ])
    collection.references.value[0]!.previewUrl = 'blob:keep'

    expect(collection.references.value.map((item) => item.key)).toEqual([
      'keep',
      'extra-0',
      'extra-1',
      'extra-2',
    ])
    expect(revoke).toHaveBeenCalledWith('blob:duplicate')
    expect(revoke).toHaveBeenCalledWith('blob:extra-3')
    const sharedPreview = createVisionReferenceCollection([
      { key: 'shared', previewUrl: 'blob:shared' },
      { key: 'shared', previewUrl: 'blob:shared' },
    ])
    expect(sharedPreview.references.value).toHaveLength(1)
    expect(revoke).not.toHaveBeenCalledWith('blob:shared')
    revoke.mockRestore()
  })

  it('normalizes preview policy on every ingress and retains durable reference identity', () => {
    const externalReference = {
      provider: 'unsplash',
      id: 'photo',
      preview_url: 'https://images.unsplash.com/original',
    }
    const valid = {
      key: 'unsplash:photo',
      externalReference,
      previewUrl: '/api/media/previews/valid/thumb',
      previewExpiresAt: new Date(Date.now() + 60_000).toISOString(),
    }
    const collection = createVisionReferenceCollection([
      { ...valid, previewUrl: externalReference.preview_url },
      { ...valid, key: 'invalid-expiry', previewExpiresAt: 'invalid' },
    ])
    collection.add({ ...valid, key: 'expired', previewExpiresAt: new Date(0).toISOString() })
    collection.add({ ...valid, key: 'valid' })
    expect(collection.references.value.map((item) => item.previewUrl)).toEqual([
      '',
      '',
      '',
      valid.previewUrl,
    ])
    expect(
      collection.references.value.every(
        (item) => item.externalReference?.preview_url === externalReference.preview_url,
      ),
    ).toBe(true)
  })

  it('expires missing external previews without touching local or Library presentation', () => {
    const revoke = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined)
    const expiresAt = Date.now() + 60_000
    vi.spyOn(Date, 'now').mockReturnValue(expiresAt - 1)
    const collection = createVisionReferenceCollection([
      {
        key: 'unsplash:photo',
        externalReference: {
          provider: 'unsplash',
          id: 'photo',
          preview_url: 'https://images.unsplash.com/input',
        },
        previewUrl: '/api/media/previews/old/thumb',
        previewExpiresAt: new Date(expiresAt).toISOString(),
      },
      { key: 'local', file: new File(['local'], 'photo.jpg'), previewUrl: 'blob:local' },
      { key: 'library', libraryAssetID: 'lasset_1', previewUrl: '/api/v1/library/thumb' },
    ])
    vi.spyOn(Date, 'now').mockReturnValue(expiresAt)
    collection.refreshPresentation([])
    expect(collection.references.value.map((item) => item.previewUrl)).toEqual([
      '',
      'blob:local',
      '/api/v1/library/thumb',
    ])
    expect(collection.persistableReferences.value.map((item) => item.key)).toEqual([
      'unsplash:photo',
      'library',
    ])
    const transferred = collection.take()
    collection.clear()
    expect(revoke).not.toHaveBeenCalled()
    const next = createVisionReferenceCollection(transferred)
    next.clear()
    expect(revoke).toHaveBeenCalledExactlyOnceWith('blob:local')
    vi.restoreAllMocks()
  })

  it('refreshes only matching external presentation and serializes expiry without provider display fallback', () => {
    const externalReference = {
      provider: 'unsplash',
      id: 'photo',
      preview_url: 'https://images.unsplash.com/input',
    }
    const collection = createVisionReferenceCollection([
      { key: 'unsplash:photo', externalReference, previewUrl: '' },
    ])
    const fresh: SearchResult = {
      provider: 'unsplash',
      id: 'photo',
      title: 'Photo',
      author: 'Author',
      author_url: '',
      source_url: '',
      thumbnail_url: '',
      original_url: 'https://images.unsplash.com/original',
      width: 100,
      height: 200,
      media_kind: 'image',
      preview_url: 'https://images.unsplash.com/replacement-input',
      display: {
        ...mediaDisplayFixture('fresh'),
        expires_at: new Date(Date.now() + 60_000).toISOString(),
      },
    }
    collection.refreshPresentation([fresh])
    expect(collection.references.value[0]?.externalReference).toEqual(externalReference)
    expect(collection.persistableReferences.value[0]).toEqual({
      key: 'unsplash:photo',
      externalReference,
      previewUrl: '/api/media/previews/fresh/thumb',
      previewExpiresAt: fresh.display?.expires_at,
    })
    collection.refreshPresentation([{ ...fresh, display: undefined }])
    expect(collection.references.value[0]?.previewUrl).toBe('')
    expect(collection.references.value[0]?.externalReference).toEqual(externalReference)
  })
})
