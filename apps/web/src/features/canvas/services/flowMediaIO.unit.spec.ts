import { describe, expect, it } from 'vitest'
import { filenameFromImageUrl, inferImageExtension, resolveFetchUrl } from './flowMediaIO'

describe('flow media URL resolution', () => {
  it('uses the same-origin upload proxy for local API upload URLs', () => {
    expect(
      resolveFetchUrl('http://localhost:6000/uploads/canvas/U123/board/content/image.png'),
    ).toBe('http://localhost:3000/uploads/canvas/U123/board/content/image.png')
  })

  it.each([
    ['image/avif', 'avif'],
    ['image/heic', 'heic'],
    ['image/heif', 'heif'],
  ])('infers %s as .%s', (mime, extension) => {
    expect(inferImageExtension(mime)).toBe(extension)
  })

  it('keeps AVIF extension when creating a stabilization filename', () => {
    expect(filenameFromImageUrl('https://cdn.example.com/image.jpg', 'image/avif')).toBe(
      'image.avif',
    )
  })
})
