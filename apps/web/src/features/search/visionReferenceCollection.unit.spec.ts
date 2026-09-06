import { describe, expect, it, vi } from 'vitest'
import { createVisionReferenceCollection, MAX_REFERENCE_COUNT } from './visionReferenceCollection'

function reference(key: string, file?: File) {
  return { key, previewUrl: `https://example.com/${key}.jpg`, file }
}

describe('visionReferenceCollection', () => {
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
})
