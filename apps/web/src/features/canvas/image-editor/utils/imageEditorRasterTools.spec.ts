import { describe, expect, it } from 'vitest'
import {
  applyImageEditorRasterStroke,
  interpolateImageEditorRasterStrokePoints,
} from './imageEditorRasterTools'

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

function createTestImageData(data: Uint8ClampedArray, width: number, height: number) {
  return new TestImageData(data, width, height) as ImageData
}

describe('imageEditorRasterTools', () => {
  it('interpolates fast pointer movement with bounded spacing', () => {
    const points = interpolateImageEditorRasterStrokePoints(
      [
        { x: 0, y: 0 },
        { x: 100, y: 0 },
      ],
      32,
    )

    expect(points.length).toBeGreaterThan(2)
    for (let index = 1; index < points.length; index += 1) {
      const previous = points[index - 1]!
      const point = points[index]!
      expect(Math.hypot(point.x - previous.x, point.y - previous.y)).toBeLessThanOrEqual(8)
    }
  })

  it('does not compound opacity when the same stroke covers a pixel repeatedly', () => {
    const imageData = createTestImageData(new Uint8ClampedArray(4), 1, 1)

    applyImageEditorRasterStroke(imageData, {
      tool: 'brush',
      points: [
        { x: 0.5, y: 0.5 },
        { x: 0.5, y: 0.5 },
        { x: 0.5, y: 0.5 },
      ],
      size: 4,
      hardness: 100,
      opacity: 50,
      color: '#ff0000',
    })

    expect(imageData.data[0]).toBe(255)
    expect(imageData.data[3]).toBe(128)
  })

  it('does not compound opacity across separated segments in the same stroke', () => {
    const imageData = createTestImageData(new Uint8ClampedArray(4), 1, 1)

    applyImageEditorRasterStroke(imageData, {
      tool: 'brush',
      points: [],
      pointGroups: [[{ x: 0.5, y: 0.5 }], [{ x: 0.5, y: 0.5 }]],
      size: 4,
      hardness: 100,
      opacity: 50,
      color: '#ff0000',
    })

    expect(imageData.data[3]).toBe(128)
  })

  it('erases to transparency according to eraser opacity', () => {
    const imageData = createTestImageData(new Uint8ClampedArray([10, 20, 30, 200]), 1, 1)

    applyImageEditorRasterStroke(imageData, {
      tool: 'eraser',
      points: [{ x: 0.5, y: 0.5 }],
      size: 4,
      hardness: 100,
      opacity: 50,
    })

    expect(imageData.data[0]).toBe(10)
    expect(imageData.data[1]).toBe(20)
    expect(imageData.data[2]).toBe(30)
    expect(imageData.data[3]).toBe(100)
  })
})
