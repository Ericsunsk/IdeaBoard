import { describe, expect, it } from 'vitest'
import { WEBP_PREVIEW_QUALITY } from '../../../../../infrastructure/media/displayImagePreviewEncoding.ts'
import {
  createImageProcessingOutputFileName,
  resolveImageProcessingOutputEncoding,
} from './imageOutputPolicy'

describe('imageOutputPolicy', () => {
  it('preserves transparency with png output', () => {
    expect(
      resolveImageProcessingOutputEncoding({
        sourceMimeType: 'image/webp',
        hasTransparentPixels: true,
      }),
    ).toEqual({ mimeType: 'image/png' })
  })

  it('uses webp and explicit quality for opaque output', () => {
    expect(
      resolveImageProcessingOutputEncoding({
        sourceMimeType: 'image/png',
        hasTransparentPixels: false,
      }),
    ).toEqual({ mimeType: 'image/webp', quality: WEBP_PREVIEW_QUALITY })
  })

  it('falls back conservatively for png when transparency is unknown', () => {
    expect(
      resolveImageProcessingOutputEncoding({
        sourceMimeType: ' IMAGE/PNG ',
        hasTransparentPixels: null,
      }),
    ).toEqual({ mimeType: 'image/png' })
  })

  it('derives adjusted filenames from the actual output mime type', () => {
    expect(
      createImageProcessingOutputFileName({
        title: 'Sample Source.png',
        mimeType: 'image/webp',
      }),
    ).toBe('Sample-Source-adjusted.webp')

    expect(
      createImageProcessingOutputFileName({
        title: 'cutout source.png',
        mimeType: 'image/png',
      }),
    ).toBe('cutout-source-adjusted.png')
  })

  it('preserves CJK filename parts and maps jpeg output to jpg', () => {
    expect(
      createImageProcessingOutputFileName({
        title: '猫 图像.jpeg',
        mimeType: 'image/jpeg',
      }),
    ).toBe('猫-图像-adjusted.jpg')
  })
})
