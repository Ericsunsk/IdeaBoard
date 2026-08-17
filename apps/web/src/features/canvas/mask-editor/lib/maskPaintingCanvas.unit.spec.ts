import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MASK_EDITOR_PAINT_RGB, MASK_EDITOR_PREVIEW_ALPHA } from '../config/maskEditorConstants'
import {
  buildMaskCommitResult,
  drawMaskRasterPreviewToCanvas,
} from './maskPaintingCanvas'

type PixelSnapshot = {
  width: number
  height: number
  data: Uint8ClampedArray
}

type CanvasCompositeOperation = 'source-over' | 'source-in' | 'destination-out'

type CanvasState = {
  fillStyle: string
  globalCompositeOperation: CanvasCompositeOperation
}

const canvasSnapshots = new WeakMap<HTMLCanvasElement, PixelSnapshot>()
const canvasContexts = new WeakMap<HTMLCanvasElement, TestCanvasContext2D>()
const encodedSnapshots = new Map<string, PixelSnapshot>()

let nextDataUrlId = 0

class TestCanvasContext2D {
  readonly canvas: HTMLCanvasElement
  fillStyle = 'rgba(0, 0, 0, 1)'
  filter = 'none'
  globalCompositeOperation: CanvasCompositeOperation = 'source-over'
  private readonly stateStack: CanvasState[] = []

  constructor(canvas: HTMLCanvasElement) {
    this.canvas = canvas
  }

  save() {
    this.stateStack.push({
      fillStyle: this.fillStyle,
      globalCompositeOperation: this.globalCompositeOperation,
    })
  }

  restore() {
    const state = this.stateStack.pop()
    if (!state) return
    this.fillStyle = state.fillStyle
    this.globalCompositeOperation = state.globalCompositeOperation
  }

  clearRect(x: number, y: number, width: number, height: number) {
    const snapshot = ensureCanvasSnapshot(this.canvas)
    forEachPixelInRect(snapshot, x, y, width, height, (base) => {
      snapshot.data[base] = 0
      snapshot.data[base + 1] = 0
      snapshot.data[base + 2] = 0
      snapshot.data[base + 3] = 0
    })
  }

  fillRect(x: number, y: number, width: number, height: number) {
    const snapshot = ensureCanvasSnapshot(this.canvas)
    const source = parseRgba(this.fillStyle)
    forEachPixelInRect(snapshot, x, y, width, height, (base) => {
      compositePixel(snapshot.data, base, source, this.globalCompositeOperation)
    })
  }

  createImageData(width: number, height: number): ImageData {
    return {
      width,
      height,
      data: new Uint8ClampedArray(width * height * 4),
    } as ImageData
  }

  getImageData(x: number, y: number, width: number, height: number): ImageData {
    const source = ensureCanvasSnapshot(this.canvas)
    const imageData = this.createImageData(width, height)
    for (let row = 0; row < height; row += 1) {
      for (let col = 0; col < width; col += 1) {
        const sourceX = x + col
        const sourceY = y + row
        if (sourceX < 0 || sourceY < 0 || sourceX >= source.width || sourceY >= source.height) {
          continue
        }
        const sourceBase = (sourceY * source.width + sourceX) * 4
        const targetBase = (row * width + col) * 4
        imageData.data.set(source.data.slice(sourceBase, sourceBase + 4), targetBase)
      }
    }
    return imageData
  }

  putImageData(imageData: ImageData, x: number, y: number) {
    const target = ensureCanvasSnapshot(this.canvas)
    for (let row = 0; row < imageData.height; row += 1) {
      for (let col = 0; col < imageData.width; col += 1) {
        const targetX = x + col
        const targetY = y + row
        if (targetX < 0 || targetY < 0 || targetX >= target.width || targetY >= target.height) {
          continue
        }
        const sourceBase = (row * imageData.width + col) * 4
        const targetBase = (targetY * target.width + targetX) * 4
        target.data.set(imageData.data.slice(sourceBase, sourceBase + 4), targetBase)
      }
    }
  }

  drawImage(image: CanvasImageSource, dx: number, dy: number, dw?: number, dh?: number) {
    if (!(image instanceof HTMLCanvasElement)) {
      throw new Error('Test canvas fixture only supports canvas image sources')
    }
    const source = ensureCanvasSnapshot(image)
    const target = ensureCanvasSnapshot(this.canvas)
    const targetWidth = Math.max(0, Math.floor(dw ?? source.width))
    const targetHeight = Math.max(0, Math.floor(dh ?? source.height))
    for (let row = 0; row < targetHeight; row += 1) {
      for (let col = 0; col < targetWidth; col += 1) {
        const targetX = Math.floor(dx + col)
        const targetY = Math.floor(dy + row)
        if (targetX < 0 || targetY < 0 || targetX >= target.width || targetY >= target.height) {
          continue
        }
        const sourceX = Math.min(source.width - 1, Math.floor((col / targetWidth) * source.width))
        const sourceY = Math.min(
          source.height - 1,
          Math.floor((row / targetHeight) * source.height),
        )
        const sourceBase = (sourceY * source.width + sourceX) * 4
        const targetBase = (targetY * target.width + targetX) * 4
        compositePixel(
          target.data,
          targetBase,
          [
            source.data[sourceBase] ?? 0,
            source.data[sourceBase + 1] ?? 0,
            source.data[sourceBase + 2] ?? 0,
            source.data[sourceBase + 3] ?? 0,
          ],
          this.globalCompositeOperation,
        )
      }
    }
  }
}

beforeEach(() => {
  encodedSnapshots.clear()
  nextDataUrlId = 0
  const createElement = document.createElement.bind(document)
  vi.spyOn(document, 'createElement').mockImplementation(((tagName: string) => {
    const element = createElement(tagName)
    if (tagName.toLowerCase() !== 'canvas') return element
    return installCanvasFixture(element as HTMLCanvasElement)
  }) as typeof document.createElement)
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('maskPaintingCanvas', () => {
  it('encodes execution masks as opaque grayscale where white redraws and black preserves', () => {
    const canvas = createFixtureCanvas(3, 1, [10, 20, 30, 0, 10, 20, 30, 128, 10, 20, 30, 255])

    const result = buildMaskCommitResult(canvas)

    expect(result).not.toBeNull()
    const executionPixels = readEncodedPixels(result!.executionMaskDataUrl)
    const previewPixels = readEncodedPixels(result!.maskDataUrl)
    expect(pixelAt(executionPixels, 0)).toEqual({ red: 0, green: 0, blue: 0, alpha: 255 })
    expect(pixelAt(previewPixels, 0).alpha).toBe(0)
    expect(pixelAt(executionPixels, 1)).toEqual({ red: 128, green: 128, blue: 128, alpha: 255 })
    expect(pixelAt(executionPixels, 2)).toEqual({ red: 255, green: 255, blue: 255, alpha: 255 })
    expect(pixelAt(previewPixels, 1)).toEqual({
      red: MASK_EDITOR_PAINT_RGB.r,
      green: MASK_EDITOR_PAINT_RGB.g,
      blue: MASK_EDITOR_PAINT_RGB.b,
      alpha: Math.round(128 * MASK_EDITOR_PREVIEW_ALPHA),
    })
    expect(pixelAt(previewPixels, 2)).toEqual({
      red: MASK_EDITOR_PAINT_RGB.r,
      green: MASK_EDITOR_PAINT_RGB.g,
      blue: MASK_EDITOR_PAINT_RGB.b,
      alpha: Math.round(255 * MASK_EDITOR_PREVIEW_ALPHA),
    })
  })

  it('treats any alpha above the empty threshold as commit-worthy mask paint', () => {
    expect(buildMaskCommitResult(createFixtureCanvas(1, 1, [0, 0, 0, 0]))).toBeNull()
    expect(buildMaskCommitResult(createFixtureCanvas(1, 1, [0, 0, 0, 1]))).not.toBeNull()
  })

  it('reloads raster mask preview with alpha greater than zero as a fully painted pixel', () => {
    const sourceImage = createFixtureCanvas(3, 1, [10, 20, 30, 0, 10, 20, 30, 1, 10, 20, 30, 128])
    const targetCanvas = createFixtureCanvas(3, 1)

    drawMaskRasterPreviewToCanvas(sourceImage, targetCanvas)

    expect(readPixels(targetCanvas)).toEqual([
      MASK_EDITOR_PAINT_RGB.r,
      MASK_EDITOR_PAINT_RGB.g,
      MASK_EDITOR_PAINT_RGB.b,
      0,
      MASK_EDITOR_PAINT_RGB.r,
      MASK_EDITOR_PAINT_RGB.g,
      MASK_EDITOR_PAINT_RGB.b,
      255,
      MASK_EDITOR_PAINT_RGB.r,
      MASK_EDITOR_PAINT_RGB.g,
      MASK_EDITOR_PAINT_RGB.b,
      255,
    ])
  })
})

function installCanvasFixture(canvas: HTMLCanvasElement): HTMLCanvasElement {
  Object.defineProperty(canvas, 'getContext', {
    configurable: true,
    value: (type: string) => {
      if (type !== '2d') return null
      let context = canvasContexts.get(canvas)
      if (!context) {
        context = new TestCanvasContext2D(canvas)
        canvasContexts.set(canvas, context)
      }
      return context
    },
  })
  Object.defineProperty(canvas, 'toDataURL', {
    configurable: true,
    value: () => {
      const snapshot = cloneSnapshot(ensureCanvasSnapshot(canvas))
      const dataUrl = `data:image/png;test-canvas=${nextDataUrlId}`
      nextDataUrlId += 1
      encodedSnapshots.set(dataUrl, snapshot)
      return dataUrl
    },
  })
  return canvas
}

function createFixtureCanvas(width: number, height: number, pixels?: number[]) {
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  const snapshot = ensureCanvasSnapshot(canvas)
  if (pixels) snapshot.data.set(pixels)
  return canvas
}

function ensureCanvasSnapshot(canvas: HTMLCanvasElement): PixelSnapshot {
  const width = canvas.width
  const height = canvas.height
  const expectedLength = width * height * 4
  const current = canvasSnapshots.get(canvas)
  if (current && current.width === width && current.height === height) return current
  const next = {
    width,
    height,
    data: new Uint8ClampedArray(expectedLength),
  }
  canvasSnapshots.set(canvas, next)
  return next
}

function cloneSnapshot(snapshot: PixelSnapshot): PixelSnapshot {
  return {
    width: snapshot.width,
    height: snapshot.height,
    data: new Uint8ClampedArray(snapshot.data),
  }
}

function readPixels(canvas: HTMLCanvasElement) {
  return Array.from(ensureCanvasSnapshot(canvas).data)
}

function readEncodedPixels(dataUrl: string) {
  const snapshot = encodedSnapshots.get(dataUrl)
  if (!snapshot) throw new Error(`Missing encoded test canvas for ${dataUrl}`)
  return Array.from(snapshot.data)
}

function pixelAt(pixels: number[], index: number) {
  const base = index * 4
  return {
    red: pixels[base],
    green: pixels[base + 1],
    blue: pixels[base + 2],
    alpha: pixels[base + 3],
  }
}

function forEachPixelInRect(
  snapshot: PixelSnapshot,
  x: number,
  y: number,
  width: number,
  height: number,
  visit: (base: number) => void,
) {
  const startX = Math.max(0, Math.floor(x))
  const startY = Math.max(0, Math.floor(y))
  const endX = Math.min(snapshot.width, Math.ceil(x + width))
  const endY = Math.min(snapshot.height, Math.ceil(y + height))
  for (let row = startY; row < endY; row += 1) {
    for (let col = startX; col < endX; col += 1) {
      visit((row * snapshot.width + col) * 4)
    }
  }
}

function parseRgba(value: string): [number, number, number, number] {
  const match = value.match(/rgba\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*([0-9.]+)\s*\)/)
  if (!match) return [0, 0, 0, 255]
  return [Number(match[1]), Number(match[2]), Number(match[3]), Math.round(Number(match[4]) * 255)]
}

function compositePixel(
  target: Uint8ClampedArray,
  base: number,
  source: [number, number, number, number],
  operation: CanvasCompositeOperation,
) {
  const [sourceRed, sourceGreen, sourceBlue, sourceAlpha] = source
  const targetAlpha = target[base + 3] ?? 0
  if (operation === 'destination-out') {
    const nextAlpha = Math.round(targetAlpha * (1 - sourceAlpha / 255))
    target[base + 3] = nextAlpha
    return
  }
  if (operation === 'source-in') {
    const nextAlpha = Math.round(sourceAlpha * (targetAlpha / 255))
    target[base] = nextAlpha > 0 ? sourceRed : 0
    target[base + 1] = nextAlpha > 0 ? sourceGreen : 0
    target[base + 2] = nextAlpha > 0 ? sourceBlue : 0
    target[base + 3] = nextAlpha
    return
  }
  target[base] = sourceRed
  target[base + 1] = sourceGreen
  target[base + 2] = sourceBlue
  target[base + 3] = sourceAlpha
}
