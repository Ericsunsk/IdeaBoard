import { describe, expect, it, vi } from 'vitest'
import { createVisionReferenceCollection, MAX_REFERENCE_BYTES } from './visionReferenceCollection'

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
    revoke.mockRestore()
  })
})
