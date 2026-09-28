import { describe, expect, it, vi } from 'vitest'
import { buildMaskCommitResult } from '../lib/maskPaintingCanvas.ts'
import {
  INPAINT_DEFAULT_BRUSH_SIZE,
  INPAINT_MAX_BRUSH_SIZE,
  MASK_PAINTING_COLOR,
} from '../config/constants.ts'
import { createFabricMaskPainter, type FabricMaskPainterDependencies } from './adapter.ts'

type RecordedPoint = { x: number; y: number }

class FakeBrush {
  color = ''
  width = 0
  down: RecordedPoint[] = []
  moves: RecordedPoint[] = []
  ups = 0
  private readonly canvas: FakeFabricCanvas
  constructor(canvas: FakeFabricCanvas) {
    this.canvas = canvas
  }

  onMouseDown(point: RecordedPoint) {
    this.down.push({ x: point.x, y: point.y })
  }

  onMouseMove(point: RecordedPoint) {
    this.moves.push({ x: point.x, y: point.y })
  }

  onMouseUp() {
    this.ups += 1
    this.canvas.objects.push({ globalCompositeOperation: 'source-over' })
  }
}

class FakeFabricCanvas {
  objects: Array<{ globalCompositeOperation: string }> = []
  backgroundImage: { globalCompositeOperation: string } | undefined = undefined
  requestRenderAll = vi.fn()
  renderAll = vi.fn()
  dispose = vi.fn()
  setDimensions = vi.fn()

  getObjects() {
    return this.objects
  }

  add(...objects: Array<{ globalCompositeOperation: string }>) {
    this.objects.push(...objects)
    return this
  }

  remove(...targets: Array<{ globalCompositeOperation: string }>) {
    this.objects = this.objects.filter((object) => !targets.includes(object))
    return this
  }
}

class FakeHostContext {
  fillStyle = ''
  globalCompositeOperation = 'source-over'
  clearRect = vi.fn((x: number, y: number, width: number, height: number) => {
    const canvas = this.canvas
    for (let row = Math.max(0, y); row < Math.min(canvas.height, y + height); row += 1) {
      for (let col = Math.max(0, x); col < Math.min(canvas.width, x + width); col += 1) {
        const base = (row * canvas.width + col) * 4
        this.pixels[base] = 0
        this.pixels[base + 1] = 0
        this.pixels[base + 2] = 0
        this.pixels[base + 3] = 0
      }
    }
  })

  constructor(canvas: { width: number; height: number }, pixels: Uint8ClampedArray) {
    this.canvas = canvas
    this.pixels = pixels
  }

  private readonly canvas: { width: number; height: number }
  private readonly pixels: Uint8ClampedArray

  getImageData() {
    return {
      width: this.canvas.width,
      height: this.canvas.height,
      data: new Uint8ClampedArray(this.pixels),
    } as ImageData
  }

  putImageData(imageData: ImageData) {
    this.pixels.set(imageData.data)
  }

  createImageData(width: number, height: number) {
    return { width, height, data: new Uint8ClampedArray(width * height * 4) } as ImageData
  }

  drawImage() {}
}

function createHost(width: number, height: number, pixels?: number[]) {
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  const buffer = new Uint8ClampedArray(width * height * 4)
  if (pixels) buffer.set(pixels)
  const context = new FakeHostContext(canvas, buffer)
  vi.spyOn(canvas, 'getContext').mockReturnValue(context as unknown as CanvasRenderingContext2D)
  const paintPixel = (index: number, alpha: number) => {
    buffer[index * 4] = 122
    buffer[index * 4 + 1] = 107
    buffer[index * 4 + 2] = 213
    buffer[index * 4 + 3] = alpha
  }
  return { canvas, context, buffer, paintPixel }
}

function createDependencies() {
  const fabricCanvas = new FakeFabricCanvas()
  const paintBrush = new FakeBrush(fabricCanvas)
  const eraseBrush = new FakeBrush(fabricCanvas)
  const baselineImage = { width: 400, height: 200, set: vi.fn() }
  const dependencies = {
    createCanvas: vi.fn(() => fabricCanvas),
    createPaintBrush: vi.fn(() => paintBrush),
    createEraseBrush: vi.fn(() => eraseBrush),
    loadBaselineImage: vi.fn(async () => baselineImage),
  } as unknown as FabricMaskPainterDependencies
  return { dependencies, fabricCanvas, paintBrush, eraseBrush, baselineImage }
}

const viewport = { left: 10, top: 20, width: 100, height: 50 }

describe('FabricMaskPainter', () => {
  it('creates one Fabric canvas with retina scaling and selection disabled', () => {
    const { canvas } = createHost(200, 100)
    const { dependencies } = createDependencies()

    createFabricMaskPainter(canvas, { width: 100, height: 50 }, dependencies)

    expect(dependencies.createCanvas).toHaveBeenCalledOnce()
    expect(dependencies.createCanvas).toHaveBeenCalledWith(
      canvas,
      expect.objectContaining({
        width: 100,
        height: 50,
        enableRetinaScaling: true,
        selection: false,
      }),
    )
  })

  it('projects viewport points into canvas-plane coordinates with clamping', () => {
    const { canvas } = createHost(200, 100)
    const deps = createDependencies()
    const painter = createFabricMaskPainter(canvas, { width: 200, height: 100 }, deps.dependencies)

    painter.paintStroke(
      [
        { x: 60, y: 45 },
        { x: -100, y: 500 },
      ],
      viewport,
    )

    // (60-10)/100*200 = 100, (45-20)/50*100 = 50; out-of-range clamps to edges.
    expect(deps.paintBrush.down).toEqual([{ x: 100, y: 50 }])
    expect(deps.paintBrush.moves).toEqual([{ x: 0, y: 100 }])
    expect(deps.paintBrush.ups).toBe(1)
  })

  it('paints single taps as dots and tints strokes with the mask color', () => {
    const { canvas } = createHost(200, 100)
    const deps = createDependencies()
    const painter = createFabricMaskPainter(canvas, { width: 200, height: 100 }, deps.dependencies)

    painter.paintStroke([{ x: 60, y: 45 }], viewport)

    expect(deps.paintBrush.down).toHaveLength(1)
    expect(deps.paintBrush.moves).toHaveLength(1)
    expect(deps.paintBrush.down[0]).toEqual(deps.paintBrush.moves[0])
    expect(deps.paintBrush.color).toBe(MASK_PAINTING_COLOR)
  })

  it('composites erase paths with destination-out and ignores empty layouts', () => {
    const { canvas } = createHost(200, 100)
    const deps = createDependencies()
    const painter = createFabricMaskPainter(canvas, { width: 200, height: 100 }, deps.dependencies)

    painter.eraseStroke(
      [
        { x: 60, y: 45 },
        { x: 70, y: 50 },
      ],
      viewport,
    )

    expect(deps.eraseBrush.ups).toBe(1)
    expect(deps.fabricCanvas.objects.at(-1)?.globalCompositeOperation).toBe('destination-out')
    expect(deps.fabricCanvas.renderAll).toHaveBeenCalled()

    painter.paintStroke([], viewport)
    painter.paintStroke([{ x: 60, y: 45 }], { left: 0, top: 0, width: 0, height: 0 })
    expect(deps.paintBrush.ups).toBe(0)
  })

  it('clamps brush size to the shared mask range on both brushes', () => {
    const { canvas } = createHost(200, 100)
    const deps = createDependencies()
    const painter = createFabricMaskPainter(canvas, { width: 200, height: 100 }, deps.dependencies)

    painter.setBrushSize(INPAINT_MAX_BRUSH_SIZE + 100)
    expect(painter.getBrushSize()).toBe(INPAINT_MAX_BRUSH_SIZE)
    painter.setBrushSize(-100)
    expect(painter.getBrushSize()).toBe(INPAINT_DEFAULT_BRUSH_SIZE)
    expect(deps.paintBrush.width).toBe(painter.getBrushSize())
    expect(deps.eraseBrush.width).toBe(painter.getBrushSize())
  })

  it('supports undo and redo across strokes at object granularity', () => {
    const { canvas } = createHost(2, 1)
    const deps = createDependencies()
    const painter = createFabricMaskPainter(canvas, { width: 2, height: 1 }, deps.dependencies)

    expect(painter.canUndo()).toBe(false)
    painter.paintStroke(
      [
        { x: 0, y: 0 },
        { x: 1, y: 0 },
      ],
      { left: 0, top: 0, width: 2, height: 1 },
    )
    // The stroke materializes as one Fabric object; undo removes the object
    // (no pixel snapshots), so a later render can never resurrect it.
    expect(deps.fabricCanvas.objects).toHaveLength(1)
    expect(painter.canUndo()).toBe(true)

    painter.undo()
    expect(deps.fabricCanvas.objects).toHaveLength(0)
    expect(deps.fabricCanvas.renderAll).toHaveBeenCalled()
    expect(painter.canUndo()).toBe(false)
    expect(painter.canRedo()).toBe(true)

    painter.redo()
    expect(deps.fabricCanvas.objects).toHaveLength(1)
    expect(painter.canRedo()).toBe(false)
  })

  it('renders live strokes incrementally with one history entry per session', () => {
    const { canvas } = createHost(2, 1)
    const deps = createDependencies()
    const painter = createFabricMaskPainter(canvas, { width: 2, height: 1 }, deps.dependencies)
    const strokeViewport = { left: 0, top: 0, width: 2, height: 1 }

    painter.beginStroke('paint', strokeViewport)
    painter.appendStrokePoints([{ x: 0, y: 0 }])
    expect(deps.paintBrush.down).toHaveLength(1)
    expect(deps.paintBrush.moves).toHaveLength(0)
    painter.appendStrokePoints([
      { x: 1, y: 0 },
      { x: 2, y: 0 },
    ])
    // Incremental: each sample drives exactly one brush move, the stroke is
    // never replayed, and nothing is committed before the session ends.
    expect(deps.paintBrush.down).toHaveLength(1)
    expect(deps.paintBrush.moves).toHaveLength(2)
    expect(deps.paintBrush.ups).toBe(0)
    expect(painter.canUndo()).toBe(false)

    painter.endStroke()
    expect(deps.paintBrush.ups).toBe(1)
    expect(deps.fabricCanvas.objects).toHaveLength(1)
    expect(painter.canUndo()).toBe(true)

    // One session is one entry: a single undo removes the whole stroke.
    painter.undo()
    expect(deps.fabricCanvas.objects).toHaveLength(0)
    expect(painter.canRedo()).toBe(true)
  })

  it('ignores live sessions without points and discards cancelled ones', () => {
    const { canvas } = createHost(2, 1)
    const deps = createDependencies()
    const painter = createFabricMaskPainter(canvas, { width: 2, height: 1 }, deps.dependencies)
    const strokeViewport = { left: 0, top: 0, width: 2, height: 1 }

    painter.beginStroke('paint', strokeViewport)
    painter.endStroke()
    expect(deps.paintBrush.ups).toBe(0)
    expect(painter.canUndo()).toBe(false)

    painter.beginStroke('erase', strokeViewport)
    painter.appendStrokePoints([{ x: 1, y: 0 }])
    painter.cancelStroke()
    expect(deps.fabricCanvas.objects).toHaveLength(0)
    expect(painter.canUndo()).toBe(false)
    expect(painter.canRedo()).toBe(false)
    expect(deps.fabricCanvas.renderAll).toHaveBeenCalled()
  })

  it('attaches the committed preview as a plane-fitted background', async () => {
    const { canvas } = createHost(200, 100)
    const deps = createDependencies()
    const painter = createFabricMaskPainter(canvas, { width: 200, height: 100 }, deps.dependencies)

    painter.setBaselineImage('data:image/png;committed')
    await vi.waitFor(() => expect(deps.fabricCanvas.backgroundImage).toBeDefined())

    // The committed bitmap is backing-store sized (400x200); it scales back
    // onto the 200x100 CSS plane instead of overflowing it.
    expect(deps.baselineImage.set).toHaveBeenCalledWith({ scaleX: 0.5, scaleY: 0.5 })
    expect(deps.fabricCanvas.renderAll).toHaveBeenCalled()

    painter.setBaselineImage(null)
    expect(deps.fabricCanvas.backgroundImage).toBeUndefined()
  })

  it('drops stale baseline loads when a newer baseline wins', async () => {
    const { canvas } = createHost(200, 100)
    const deps = createDependencies()
    let resolveLoad!: (image: { width: number; height: number; set: () => void }) => void
    const gate = new Promise<{ width: number; height: number; set: () => void }>((resolve) => {
      resolveLoad = resolve
    })
    const gated = {
      ...deps,
      dependencies: {
        ...(deps.dependencies as unknown as Record<string, unknown>),
        loadBaselineImage: () => gate,
      },
    } as unknown as { dependencies: FabricMaskPainterDependencies }

    const painter = createFabricMaskPainter(canvas, { width: 200, height: 100 }, gated.dependencies)
    painter.setBaselineImage('data:image/png;stale')
    painter.setBaselineImage(null)
    resolveLoad({ width: 400, height: 200, set: () => {} })
    await gate
    await Promise.resolve()

    expect(deps.fabricCanvas.backgroundImage).toBeUndefined()
  })

  it('clears strokes and baseline together with object-granular undo', async () => {
    const { canvas, paintPixel } = createHost(2, 1)
    const deps = createDependencies()
    const painter = createFabricMaskPainter(canvas, { width: 2, height: 1 }, deps.dependencies)

    painter.setBaselineImage('data:image/png;committed')
    await vi.waitFor(() => expect(deps.fabricCanvas.backgroundImage).toBeDefined())
    paintPixel(1, 200)
    painter.paintStroke(
      [
        { x: 0, y: 0 },
        { x: 1, y: 0 },
      ],
      { left: 0, top: 0, width: 2, height: 1 },
    )

    painter.clear()
    expect(deps.fabricCanvas.objects).toHaveLength(0)
    expect(deps.fabricCanvas.backgroundImage).toBeUndefined()
    expect(painter.canUndo()).toBe(true)

    painter.undo()
    expect(deps.fabricCanvas.objects).toHaveLength(1)
    expect(deps.fabricCanvas.backgroundImage).toBeDefined()

    painter.redo()
    expect(deps.fabricCanvas.objects).toHaveLength(0)
    expect(deps.fabricCanvas.backgroundImage).toBeUndefined()
  })

  it('heals a viewport-reset backing store on hidpi screens', () => {
    const { canvas } = createHost(200, 100)
    const deps = createDependencies()
    const previousRatio = window.devicePixelRatio
    Object.defineProperty(window, 'devicePixelRatio', { configurable: true, value: 2 })
    try {
      const painter = createFabricMaskPainter(canvas, { width: 200, height: 100 }, deps.dependencies)
      // Vue owns the width/height attrs (CSS plane); a backing store that
      // collapsed to the CSS size is re-applied as Fabric retina dimensions.
      expect(canvas.width).toBe(200)
      painter.beginStroke('paint', { left: 0, top: 0, width: 200, height: 100 })
      expect(deps.fabricCanvas.setDimensions).toHaveBeenCalledWith({
        width: 200,
        height: 100,
      })
    } finally {
      Object.defineProperty(window, 'devicePixelRatio', { configurable: true, value: previousRatio })
    }
  })

  it('clears only when painted and reports paint state', () => {
    const { canvas, paintPixel } = createHost(2, 1)
    const deps = createDependencies()
    const painter = createFabricMaskPainter(canvas, { width: 2, height: 1 }, deps.dependencies)

    expect(painter.hasPaint()).toBe(false)
    painter.clear()
    expect(painter.canUndo()).toBe(false)

    // Rendered pixels (strokes or baseline) still gate the no-op clear.
    paintPixel(1, 200)
    expect(painter.hasPaint()).toBe(true)
  })

  it('reports paint state from the flag without scanning after paint', () => {
    const { canvas } = createHost(2, 1)
    const deps = createDependencies()
    const painter = createFabricMaskPainter(canvas, { width: 2, height: 1 }, deps.dependencies)
    const getContext = canvas.getContext as unknown as { mockClear: () => void; mock: { calls: unknown[] } }

    painter.paintStroke(
      [
        { x: 0, y: 0 },
        { x: 1, y: 0 },
      ],
      { left: 0, top: 0, width: 2, height: 1 },
    )
    getContext.mockClear()

    // The painted-state flag answers without a full-canvas pixel scan.
    expect(painter.hasPaint()).toBe(true)
    expect(getContext.mock.calls).toHaveLength(0)
  })

  it('clears after paint without rescanning and resets the flag on undo-to-empty', () => {
    const { canvas } = createHost(2, 1)
    const deps = createDependencies()
    const painter = createFabricMaskPainter(canvas, { width: 2, height: 1 }, deps.dependencies)
    const getContext = canvas.getContext as unknown as { mockClear: () => void; mock: { calls: unknown[] } }

    painter.paintStroke(
      [
        { x: 0, y: 0 },
        { x: 1, y: 0 },
      ],
      { left: 0, top: 0, width: 2, height: 1 },
    )
    getContext.mockClear()

    painter.clear()
    expect(painter.canUndo()).toBe(true)
    expect(getContext.mock.calls).toHaveLength(0)

    // Undoing the clear restores the stroke; undoing the stroke empties the
    // stack, which clears the flag.
    painter.undo()
    expect(painter.canUndo()).toBe(true)
    painter.undo()
    expect(painter.canUndo()).toBe(false)
    // Undo-to-empty clears the flag: the next read falls back to the scan.
    getContext.mockClear()
    expect(painter.hasPaint()).toBe(false)
    expect(getContext.mock.calls.length).toBeGreaterThan(0)
  })

  it('commits dual mask data identical to the legacy builder for the same pixels', () => {
    const painted = [122, 107, 213, 0, 122, 107, 213, 128, 122, 107, 213, 255]
    const first = createHost(3, 1, painted)
    const second = createHost(3, 1, painted)
    const deps = createDependencies()
    const painter = createFabricMaskPainter(first.canvas, { width: 3, height: 1 }, deps.dependencies)

    const committed = withDeterministicTempCanvases(() => painter.commit())
    const legacy = withDeterministicTempCanvases(() => buildMaskCommitResult(second.canvas))

    expect(committed).not.toBeNull()
    expect(committed).toEqual(legacy)
    expect(committed?.executionMaskDataUrl).not.toBe(committed?.maskDataUrl)
  })

  it('commits null when nothing is painted', () => {
    const { canvas } = createHost(2, 1)
    const deps = createDependencies()
    const painter = createFabricMaskPainter(canvas, { width: 2, height: 1 }, deps.dependencies)

    expect(painter.commit()).toBeNull()
  })

  it('disposes the Fabric canvas and ignores later work', () => {
    const { canvas } = createHost(2, 1)
    const deps = createDependencies()
    const painter = createFabricMaskPainter(canvas, { width: 2, height: 1 }, deps.dependencies)

    painter.dispose()

    expect(deps.fabricCanvas.dispose).toHaveBeenCalledOnce()
    painter.paintStroke([{ x: 1, y: 0 }], { left: 0, top: 0, width: 2, height: 1 })
    painter.clear()
    painter.undo()
    expect(painter.commit()).toBeNull()
    expect(deps.paintBrush.ups).toBe(0)
  })
})

/**
 * Installs deterministic temp canvases for the legacy commit builder (it
 * allocates scratch canvases via `document.createElement`), so the Fabric
 * commit output can be compared pixel-contract for pixel-contract.
 */
function withDeterministicTempCanvases<T>(run: () => T): T {
  let nextId = 0
  const createElement = document.createElement.bind(document)
  const spy = vi.spyOn(document, 'createElement').mockImplementation(((tagName: string) => {
    const element = createElement(tagName)
    if (tagName.toLowerCase() !== 'canvas') return element
    return installDeterministicCanvas(element as HTMLCanvasElement, () => nextId++)
  }) as typeof document.createElement)
  try {
    return run()
  } finally {
    spy.mockRestore()
  }
}

function installDeterministicCanvas(
  target: HTMLCanvasElement,
  nextId: () => number,
): HTMLCanvasElement {
  const pixels = new Uint8ClampedArray(3 * 4)
  const context: {
    fillStyle: string
    globalCompositeOperation: string
    save(): void
    restore(): void
    clearRect(): void
    createImageData(width: number, height: number): ImageData
    getImageData(): ImageData
    putImageData(imageData: ImageData): void
    drawImage(source: HTMLCanvasElement): void
    fillRect(x: number, y: number, width: number, height: number): void
  } = {
    fillStyle: '',
    globalCompositeOperation: 'source-over',
    save() {},
    restore() {},
    clearRect() {
      pixels.fill(0)
    },
    createImageData(width: number, height: number) {
      return { width, height, data: new Uint8ClampedArray(width * height * 4) } as ImageData
    },
    getImageData() {
      return { width: 3, height: 1, data: new Uint8ClampedArray(pixels) } as ImageData
    },
    putImageData(imageData: ImageData) {
      pixels.set(imageData.data)
    },
    drawImage(source: HTMLCanvasElement) {
      const sourceContext = source.getContext('2d') as unknown as { getImageData(): ImageData }
      pixels.set(sourceContext.getImageData().data)
    },
    fillRect(_x: number, _y: number, width: number, height: number) {
      if (width <= 0 || height <= 0) return
      const match = /rgba\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*([0-9.]+)\s*\)/.exec(
        context.fillStyle,
      )
      if (!match) return
      const alpha = Math.round(Number(match[4]) * 255)
      for (let index = 0; index < 3; index += 1) {
        const base = index * 4
        if (context.globalCompositeOperation === 'source-in') {
          const nextAlpha = Math.round((alpha * (pixels[base + 3] ?? 0)) / 255)
          pixels[base] = nextAlpha > 0 ? Number(match[1]) : 0
          pixels[base + 1] = nextAlpha > 0 ? Number(match[2]) : 0
          pixels[base + 2] = nextAlpha > 0 ? Number(match[3]) : 0
          pixels[base + 3] = nextAlpha
        } else {
          pixels[base] = Number(match[1])
          pixels[base + 1] = Number(match[2])
          pixels[base + 2] = Number(match[3])
          pixels[base + 3] = alpha
        }
      }
    },
  }
  Object.defineProperty(target, 'getContext', {
    configurable: true,
    value: () => context,
  })
  Object.defineProperty(target, 'toDataURL', {
    configurable: true,
    value: () => `data:image/png;fixture=${nextId()}:${Array.from(pixels).join(',')}`,
  })
  return target
}
