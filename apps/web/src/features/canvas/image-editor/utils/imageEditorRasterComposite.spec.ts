import { describe, expect, it } from 'vitest'
import type { ImageEditorLayer, ImageEditorRasterContent } from '../types'
import { compositeImageEditorLayerStack } from './imageEditorRasterComposite'

const rasterContent: ImageEditorRasterContent = {
  kind: 'raster',
  dataUrl: 'data:image/png;base64,paint',
  width: 1,
  height: 1,
}

class TestImageData {
  data: Uint8ClampedArray
  width: number
  height: number

  constructor(data: Uint8ClampedArray, width: number, height: number) {
    this.data = data
    this.width = width
    this.height = height
  }
}

function createPixelImageData(r: number, g: number, b: number, a: number) {
  return new TestImageData(new Uint8ClampedArray([r, g, b, a]), 1, 1) as ImageData
}

describe('imageEditorRasterComposite', () => {
  it('alpha-composites visible paint layers over the source', () => {
    const output = compositeImageEditorLayerStack(
      createPixelImageData(255, 0, 0, 255),
      [
        {
          id: 'paint:1',
          type: 'paint',
          name: '图层 1',
          visible: true,
          content: rasterContent,
        },
        { id: 'source', type: 'source', name: '原图', visible: true },
      ],
      {
        getRasterImageData: () => createPixelImageData(0, 0, 255, 128),
      },
    )

    expect(Array.from(output.data)).toEqual([127, 0, 128, 255])
  })

  it('applies adjustment layers only to the composited result below them', () => {
    const layers: ImageEditorLayer[] = [
      {
        id: 'paint:1',
        type: 'paint',
        name: '图层 1',
        visible: true,
        content: rasterContent,
      },
      {
        id: 'adjustment:threshold',
        type: 'adjustment',
        kind: 'threshold',
        name: '阈值 1',
        visible: true,
        params: { threshold: 128, invert: false },
      },
      { id: 'source', type: 'source', name: '原图', visible: true },
    ]

    const output = compositeImageEditorLayerStack(createPixelImageData(20, 20, 20, 255), layers, {
      getRasterImageData: () => createPixelImageData(0, 0, 255, 255),
    })

    expect(Array.from(output.data)).toEqual([0, 0, 255, 255])
  })
})
