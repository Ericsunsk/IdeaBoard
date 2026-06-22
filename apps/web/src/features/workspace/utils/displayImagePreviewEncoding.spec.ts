import { describe, expect, it } from 'vitest'
import {
  imageDataHasTransparentPixels,
  resolvePreviewMimeType,
} from './displayImagePreviewEncoding'

describe('displayImagePreviewEncoding', () => {
  it('detects transparent and semitransparent pixels from alpha values', () => {
    expect(
      imageDataHasTransparentPixels(
        new Uint8ClampedArray([
          255, 255, 255, 255,
          255, 255, 255, 0,
        ]),
      ),
    ).toBe(true)

    expect(
      imageDataHasTransparentPixels(
        new Uint8ClampedArray([
          255, 255, 255, 255,
          0, 0, 0, 254,
        ]),
      ),
    ).toBe(true)

    expect(
      imageDataHasTransparentPixels(
        new Uint8ClampedArray([
          255, 255, 255, 255,
          0, 0, 0, 255,
        ]),
      ),
    ).toBe(false)
  })

  it('uses png previews only when transparency must be preserved', () => {
    expect(resolvePreviewMimeType('image/png', true)).toBe('image/png')
    expect(resolvePreviewMimeType('image/webp', true)).toBe('image/png')
    expect(resolvePreviewMimeType('image/png', false)).toBe('image/webp')
    expect(resolvePreviewMimeType('image/jpeg', false)).toBe('image/webp')
  })

  it('falls back conservatively when png transparency cannot be inspected', () => {
    expect(resolvePreviewMimeType(' IMAGE/PNG ', null)).toBe('image/png')
    expect(resolvePreviewMimeType('image/jpeg', null)).toBe('image/webp')
  })
})
