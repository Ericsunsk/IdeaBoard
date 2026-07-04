import { describe, expect, it } from 'vitest'
import { isAnimatedImageBlob } from './imageAdjustmentMedia'

const ANIMATED_WEBP_BYTES = new Uint8Array([
  0x52, 0x49, 0x46, 0x46, 0x12, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50, 0x41, 0x4e, 0x49,
  0x4d, 0x00, 0x00, 0x00, 0x00,
])

const ANIMATED_PNG_BYTES = new Uint8Array([
  0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
  0x00, 0x00, 0x00, 0x08,
  0x61, 0x63, 0x54, 0x4c,
  0x00, 0x00, 0x00, 0x02,
  0x00, 0x00, 0x00, 0x00,
  0x00, 0x00, 0x00, 0x00,
])

describe('imageAdjustmentMedia', () => {
  it('detects animated image blobs before bitmap decoding', async () => {
    await expect(
      isAnimatedImageBlob(new Blob(['source'], { type: 'image/gif' })),
    ).resolves.toBe(true)
    await expect(
      isAnimatedImageBlob(new Blob([ANIMATED_WEBP_BYTES], { type: 'image/webp' })),
    ).resolves.toBe(true)
    await expect(
      isAnimatedImageBlob(new Blob([ANIMATED_PNG_BYTES], { type: 'image/png' })),
    ).resolves.toBe(true)
  })

  it('keeps still images eligible for adjustment', async () => {
    await expect(
      isAnimatedImageBlob(new Blob(['source'], { type: 'image/jpeg' })),
    ).resolves.toBe(false)
    await expect(
      isAnimatedImageBlob(new Blob(['not-a-webp'], { type: 'image/webp' })),
    ).resolves.toBe(false)
  })
})
