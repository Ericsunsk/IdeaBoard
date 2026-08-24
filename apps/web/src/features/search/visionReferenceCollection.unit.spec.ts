import { describe, expect, it, vi } from 'vitest'
import {
  createVisionReferenceCollection,
  MAX_REFERENCE_BYTES,
  MAX_REFERENCE_COUNT,
} from './visionReferenceCollection'

function reference(key: string, file?: File) {
  return { key, previewUrl: `https://example.com/${key}.jpg`, file }
}

describe('visionReferenceCollection', () => {
  it('deduplicates references and counts pending slots', () => {
    const collection = createVisionReferenceCollection()
    expect(collection.add(reference('a'))).toBe(true)
    expect(collection.add(reference('a'))).toBe(false)
    expect(collection.begin('b')).toBe(true)
    expect(collection.slotCount.value).toBe(2)
    expect(collection.hasPending.value).toBe(true)
    expect(collection.add(reference('b'))).toBe(false)
  })

  it('reserves bytes before materializing a remote reference', () => {
    const collection = createVisionReferenceCollection()
    expect(collection.begin('remote')).toBe(true)
    expect(collection.appendPreview('remote', 'https://example.com/remote.jpg')).toBe(true)
    expect(collection.reserveBytes('remote', MAX_REFERENCE_BYTES + 1)).toBe(false)
    expect(collection.reserveBytes('remote', 1024)).toBe(true)
    expect(collection.materialize('remote', new File(['image'], 'remote.jpg', { type: 'image/jpeg' }))).toBe(true)
    expect(collection.hasPending.value).toBe(true)
    collection.finish('remote')
    expect(collection.hasPending.value).toBe(false)
    expect(collection.references.value[0]?.file).toBeInstanceOf(File)
  })

  it('removes local object URLs and rolls back failed references', () => {
    const revoke = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined)
    const collection = createVisionReferenceCollection([
      reference('local', new File(['image'], 'local.jpg', { type: 'image/jpeg' })),
    ])
    collection.references.value[0]!.previewUrl = 'blob:local'

    expect(collection.remove('local')).toBe(true)
    expect(revoke).toHaveBeenCalledWith('blob:local')

    collection.begin('remote')
    collection.appendPreview('remote', 'blob:remote')
    expect(collection.fail('remote')).toBe(true)
    expect(collection.references.value).toHaveLength(0)
    expect(revoke).toHaveBeenCalledWith('blob:remote')
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
