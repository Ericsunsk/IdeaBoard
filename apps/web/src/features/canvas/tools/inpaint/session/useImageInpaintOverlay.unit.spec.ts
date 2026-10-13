import type { Node } from '@vue-flow/core'
import { describe, expect, it, vi } from 'vitest'
import { computed, nextTick, ref, shallowRef } from 'vue'
import type { Ref } from 'vue'
import { useRetouchMode } from '../../retouch'
import { INPAINT_DEFAULT_BRUSH_SIZE, INPAINT_MAX_BRUSH_SIZE } from '../core/constants'
import {
  INPAINT_MAX_PENDING_STROKES_PER_NODE,
  INPAINT_MAX_POINTS_PER_STROKE,
} from '../core/constants'
import type { MaskPainter, MaskPainterClientPoint, MaskPainterViewport } from '../core/painter.ts'
import type { ImageInpaintCommittedData } from '../core/model.ts'
import { useImageInpaintOverlay } from './useImageInpaintOverlay'

type RecordedStroke = {
  mode: 'paint' | 'erase'
  points: MaskPainterClientPoint[]
  viewport: MaskPainterViewport
}

class FakeMaskPainter implements MaskPainter {
  strokes: RecordedStroke[] = []
  liveBegins: Array<{ mode: 'paint' | 'erase'; viewport: MaskPainterViewport }> = []
  appendedBatches: MaskPainterClientPoint[][] = []
  baselines: Array<string | null> = []
  brushSize = INPAINT_DEFAULT_BRUSH_SIZE
  setBrushSizeCalls: number[] = []
  undoCalls = 0
  redoCalls = 0
  clearCalls = 0
  endCalls = 0
  cancelCalls = 0
  disposed = false
  private livePoints: MaskPainterClientPoint[] = []
  private painted = false
  private readonly undoStack: number[] = []
  private readonly redoStack: number[] = []

  paintStroke(points: readonly MaskPainterClientPoint[], viewport: MaskPainterViewport) {
    this.strokes.push({ mode: 'paint', points: [...points], viewport: { ...viewport } })
    this.undoStack.push(1)
    this.redoStack.length = 0
    this.painted = true
  }

  eraseStroke(points: readonly MaskPainterClientPoint[], viewport: MaskPainterViewport) {
    this.strokes.push({ mode: 'erase', points: [...points], viewport: { ...viewport } })
    this.undoStack.push(1)
    this.redoStack.length = 0
    this.painted = true
  }

  beginStroke(mode: 'paint' | 'erase', viewport: MaskPainterViewport) {
    this.liveBegins.push({ mode, viewport: { ...viewport } })
    this.livePoints = []
  }

  appendStrokePoints(points: readonly MaskPainterClientPoint[]) {
    this.appendedBatches.push([...points])
    this.livePoints.push(...points)
  }

  endStroke() {
    this.endCalls += 1
    if (this.livePoints.length > 0) {
      this.undoStack.push(1)
      this.redoStack.length = 0
      this.painted = true
    }
    this.livePoints = []
  }

  cancelStroke() {
    this.cancelCalls += 1
    this.livePoints = []
  }

  setBaselineImage(dataUrl: string | null) {
    this.baselines.push(dataUrl)
  }

  setBrushSize(size: number) {
    this.setBrushSizeCalls.push(size)
    this.brushSize = size
  }

  getBrushSize() {
    return this.brushSize
  }

  canUndo() {
    return this.undoStack.length > 0
  }

  canRedo() {
    return this.redoStack.length > 0
  }

  undo() {
    this.undoCalls += 1
    if (this.undoStack.pop() !== undefined) this.redoStack.push(1)
  }

  redo() {
    this.redoCalls += 1
    if (this.redoStack.pop() !== undefined) this.undoStack.push(1)
  }

  clear() {
    this.clearCalls += 1
    if (this.painted) this.undoStack.push(1)
    this.strokes.length = 0
    this.livePoints = []
    this.painted = false
    this.redoStack.length = 0
  }

  hasPaint() {
    return this.painted
  }

  commit(): ImageInpaintCommittedData | null {
    if (!this.painted) return null
    return {
      executionMaskDataUrl: 'data:image/png;fake-execution',
      maskDataUrl: 'data:image/png;fake-preview',
    }
  }

  dispose() {
    this.disposed = true
  }
}

function createStageElement() {
  const stage = document.createElement('div')
  Object.defineProperties(stage, {
    clientWidth: { value: 320 },
    clientHeight: { value: 240 },
  })
  return stage
}

function createHarness() {
  const stage = createStageElement()
  const painters: FakeMaskPainter[] = []
  const createMaskPainter = vi.fn((host: HTMLCanvasElement): MaskPainter | Promise<MaskPainter> => {
    void host
    const painter = new FakeMaskPainter()
    painters.push(painter)
    return painter
  })
  const nodes: Ref<Node[]> = shallowRef([{ id: 'node-1', data: { kind: 'image' } }] as Node[])
  const editorMode = useRetouchMode({
    nodes,
    activeTool: ref('select'),
    getImageNodeDisplayPayload: (nodeId) =>
      nodeId === 'node-1' ? { url: 'https://example.com/source.png', ratio: 1 } : null,
  })
  const overlay = useImageInpaintOverlay({
    inpaintEditorState: computed(() => {
      const state = editorMode.retouchState.value
      return state?.mode === 'inpaint' ? { nodeId: state.nodeId, mode: 'inpaint' as const } : null
    }),
    imageRect: computed(() => ({ left: 40, top: 0, width: stage.clientWidth - 80, height: 240 })),
    createMaskPainter,
  })

  return { editorMode, overlay, painters, createMaskPainter }
}

function createMaskCanvas() {
  const canvas = document.createElement('canvas')
  canvas.width = 200
  canvas.height = 100
  const ctx = {
    clearRect: vi.fn(),
    drawImage: vi.fn(),
    getImageData: vi.fn(),
    putImageData: vi.fn(),
  } as unknown as CanvasRenderingContext2D
  vi.spyOn(canvas, 'getContext').mockReturnValue(ctx)
  return canvas
}

async function mountPainter(
  overlay: ReturnType<typeof useImageInpaintOverlay>,
  canvas: HTMLCanvasElement,
) {
  overlay.setMaskCanvasRef('node-1', canvas)
  await Promise.resolve()
  await nextTick()
}

function pointerDown(init: PointerEventInit = {}) {
  return new PointerEvent('pointerdown', {
    button: 0,
    clientX: 60,
    clientY: 45,
    pointerId: 1,
    pointerType: 'mouse',
    ...init,
  })
}

/**
 * Canvas with a working 2d double for the committed-preview pixel path:
 * the overlay reads the target context and `drawMaskRasterPreviewToCanvas`
 * allocates scratch canvases through `document.createElement`.
 */
function createPixelCanvas(width: number, height: number) {
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  const context = {
    clearRect: vi.fn(),
    drawImage: vi.fn(),
    getImageData: vi.fn(
      () =>
        ({
          width,
          height,
          data: new Uint8ClampedArray(width * height * 4),
        }) as unknown as ImageData,
    ),
    createImageData: vi.fn(
      (w: number, h: number) =>
        ({ width: w, height: h, data: new Uint8ClampedArray(w * h * 4) }) as unknown as ImageData,
    ),
    putImageData: vi.fn(),
  }
  vi.spyOn(canvas, 'getContext').mockReturnValue(context as unknown as CanvasRenderingContext2D)
  return { canvas, context }
}

/**
 * Scratch canvases from `drawMaskRasterPreviewToCanvas` get an opaque
 * bitmap so the tinted preview reaches the target `putImageData`.
 */
function stubCanvasElementCreation() {
  const realCreateElement = document.createElement.bind(document)
  const spy = vi.spyOn(document, 'createElement')
  spy.mockImplementation(((tagName: string) => {
    if (tagName.toLowerCase() !== 'canvas') return realCreateElement(tagName)
    const temp = realCreateElement('canvas') as HTMLCanvasElement
    const tempContext = {
      clearRect: vi.fn(),
      drawImage: vi.fn(),
      getImageData: vi.fn(() => {
        const data = new Uint8ClampedArray(temp.width * temp.height * 4)
        for (let index = 3; index < data.length; index += 4) data[index] = 255
        return { width: temp.width, height: temp.height, data }
      }),
    }
    vi.spyOn(temp, 'getContext').mockReturnValue(tempContext as unknown as CanvasRenderingContext2D)
    return temp
  }) as unknown as typeof document.createElement)
  return spy
}

describe('useImageInpaintOverlay', () => {
  it('keeps mask editor active when committing without a drawable mask', async () => {
    const { editorMode, overlay } = createHarness()
    const canvas = createMaskCanvas()
    canvas.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    await mountPainter(overlay, canvas)
    expect(overlay.commitMaskDraft()).toBe(false)

    expect(editorMode.retouchState.value).not.toBeNull()
    expect(overlay.inpaintNodeId.value).toBe('node-1')
  })

  it('reuses the pointer-down canvas layout while painting incrementally', async () => {
    const { editorMode, overlay, painters } = createHarness()
    const canvas = createMaskCanvas()
    const readRect = vi.fn(() => ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect)
    canvas.getBoundingClientRect = readRect

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    await mountPainter(overlay, canvas)
    overlay.onCanvasPointerDown(pointerDown())
    window.dispatchEvent(
      new PointerEvent('pointermove', {
        clientX: 70,
        clientY: 50,
        pointerId: 1,
        pointerType: 'mouse',
      }),
    )

    expect(readRect).toHaveBeenCalledTimes(1)
    window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, pointerType: 'mouse' }))

    const painter = painters.at(-1)
    // Incremental live preview: one begin, per-move appends, one end; the
    // release path must not replay the buffered points as a second stroke.
    expect(painter?.liveBegins).toEqual([
      { mode: 'paint', viewport: { left: 10, top: 20, width: 100, height: 50 } },
    ])
    expect(painter?.appendedBatches).toEqual([[{ x: 60, y: 45 }], [{ x: 70, y: 50 }]])
    expect(painter?.endCalls).toBe(1)
    expect(painter?.cancelCalls).toBe(0)
    expect(painter?.strokes).toHaveLength(0)
    expect(overlay.canUndoMask.value).toBe(true)
  })

  it('paints a single tap as one live stroke so the seam can render its dot', async () => {
    const { editorMode, overlay, painters } = createHarness()
    const canvas = createMaskCanvas()
    canvas.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    await mountPainter(overlay, canvas)
    overlay.onCanvasPointerDown(pointerDown())
    window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, pointerType: 'mouse' }))

    // Visual parity with the legacy tap dot: the down sample opens the live
    // session, so a tap with zero moves still reaches the seam exactly once.
    const painter = painters.at(-1)
    expect(painter?.liveBegins).toHaveLength(1)
    expect(painter?.appendedBatches).toEqual([[{ x: 60, y: 45 }]])
    expect(painter?.endCalls).toBe(1)
    expect(painter?.strokes).toHaveLength(0)
  })

  it('discards the live preview when the drag is cancelled', async () => {
    const { editorMode, overlay, painters } = createHarness()
    const canvas = createMaskCanvas()
    canvas.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    await mountPainter(overlay, canvas)
    overlay.onCanvasPointerDown(pointerDown())
    window.dispatchEvent(
      new PointerEvent('pointermove', {
        clientX: 70,
        clientY: 50,
        pointerId: 1,
        pointerType: 'mouse',
      }),
    )
    window.dispatchEvent(new PointerEvent('pointercancel', { pointerId: 1, pointerType: 'mouse' }))

    const painter = painters.at(-1)
    expect(painter?.cancelCalls).toBe(1)
    expect(painter?.endCalls).toBe(0)
    expect(painter?.strokes).toHaveLength(0)
    expect(overlay.canUndoMask.value).toBe(false)
  })

  it('falls back to one-shot release when the painter loads mid-drag', async () => {
    const { editorMode, overlay, painters, createMaskPainter } = createHarness()
    const canvas = createMaskCanvas()
    canvas.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect

    let resolvePainter!: (painter: FakeMaskPainter) => void
    createMaskPainter.mockImplementationOnce(
      () =>
        new Promise<FakeMaskPainter>((resolve) => {
          resolvePainter = resolve
        }),
    )

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    overlay.setMaskCanvasRef('node-1', canvas)
    await Promise.resolve()
    await nextTick()
    // The painter is still loading: the drag only buffers, no live session.
    overlay.onCanvasPointerDown(pointerDown())
    window.dispatchEvent(
      new PointerEvent('pointermove', {
        clientX: 70,
        clientY: 50,
        pointerId: 1,
        pointerType: 'mouse',
      }),
    )
    window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, pointerType: 'mouse' }))

    const latePainter = new FakeMaskPainter()
    painters.push(latePainter)
    resolvePainter(latePainter)
    await Promise.resolve()
    await nextTick()

    expect(latePainter.liveBegins).toHaveLength(0)
    expect(latePainter.strokes).toEqual([
      {
        mode: 'paint',
        points: [
          { x: 60, y: 45 },
          { x: 70, y: 50 },
        ],
        viewport: { left: 10, top: 20, width: 100, height: 50 },
      },
    ])
  })

  it('drops pointer pressure so strokes render at constant brush width', async () => {
    const { editorMode, overlay, painters } = createHarness()
    const canvas = createMaskCanvas()
    canvas.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    await mountPainter(overlay, canvas)
    overlay.setBrushSize(24)
    overlay.onCanvasPointerDown(pointerDown({ pressure: 0.2 }))
    window.dispatchEvent(
      new PointerEvent('pointermove', {
        clientX: 70,
        clientY: 50,
        pointerId: 1,
        pointerType: 'mouse',
        pressure: 0.9,
      }),
    )
    window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, pointerType: 'mouse' }))

    // Legacy parity: the old engine ran with thinning 0 (pressure-invariant
    // width); the seam receives x/y only and one clamped brush size.
    const painter = painters.at(-1)
    expect(painter?.appendedBatches.flat()).toEqual([
      { x: 60, y: 45 },
      { x: 70, y: 50 },
    ])
    expect(painter?.setBrushSizeCalls.at(-1)).toBe(24)
  })

  it('routes erase strokes to the seam live path', async () => {
    const { editorMode, overlay, painters } = createHarness()
    const canvas = createMaskCanvas()
    canvas.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    await mountPainter(overlay, canvas)
    overlay.setDrawMode('erase')
    overlay.onCanvasPointerDown(pointerDown())
    window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, pointerType: 'mouse' }))

    expect(painters.at(-1)?.liveBegins[0]?.mode).toBe('erase')
    expect(painters.at(-1)?.endCalls).toBe(1)
    expect(painters.at(-1)?.strokes).toHaveLength(0)
  })

  it('restores the committed preview through the seam baseline on remount', async () => {
    const { editorMode, overlay, painters } = createHarness()
    const canvas = createMaskCanvas()
    canvas.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    await mountPainter(overlay, canvas)
    overlay.onCanvasPointerDown(pointerDown())
    window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, pointerType: 'mouse' }))
    expect(overlay.commitMaskDraft()).toBe(true)

    overlay.setMaskCanvasRef('node-1', null)
    overlay.setMaskCanvasRef('node-1', canvas)
    await Promise.resolve()
    await nextTick()

    expect(painters.at(-1)?.baselines.at(-1)).toBe('data:image/png;fake-preview')
  })

  it('delegates undo, redo, clear, and commit to the seam painter', async () => {
    const { editorMode, overlay, painters } = createHarness()
    const canvas = createMaskCanvas()
    canvas.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    await mountPainter(overlay, canvas)
    overlay.onCanvasPointerDown(pointerDown())
    window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, pointerType: 'mouse' }))

    const painter = painters.at(-1)
    expect(painter).toBeDefined()
    expect(overlay.canUndoMask.value).toBe(true)
    expect(overlay.commitMaskDraft()).toBe(true)
    expect(overlay.getCommittedMaskData('node-1')).toEqual({
      executionMaskDataUrl: 'data:image/png;fake-execution',
      maskDataUrl: 'data:image/png;fake-preview',
    })

    overlay.undoMask()
    expect(painter?.undoCalls).toBe(1)
    expect(overlay.canUndoMask.value).toBe(false)
    expect(overlay.canRedoMask.value).toBe(true)

    overlay.redoMask()
    expect(painter?.redoCalls).toBe(1)

    overlay.clearMask()
    expect(painter?.clearCalls).toBe(1)
    expect(overlay.commitMaskDraft()).toBe(false)
  })

  it('clamps brush size through the seam range', async () => {
    const { editorMode, overlay, painters } = createHarness()
    const canvas = createMaskCanvas()
    canvas.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    await mountPainter(overlay, canvas)
    overlay.setBrushSize(INPAINT_MAX_BRUSH_SIZE + 100)
    expect(overlay.brushSize.value).toBe(INPAINT_MAX_BRUSH_SIZE)

    overlay.onCanvasPointerDown(pointerDown())
    window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, pointerType: 'mouse' }))
    expect(painters.at(-1)?.setBrushSizeCalls.at(-1)).toBe(INPAINT_MAX_BRUSH_SIZE)
  })

  it('caps buffered strokes while the painter loads, keeping the newest', async () => {
    const { editorMode, overlay, painters, createMaskPainter } = createHarness()
    const canvas = createMaskCanvas()
    canvas.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect

    let resolvePainter!: (painter: FakeMaskPainter) => void
    createMaskPainter.mockImplementationOnce(
      () =>
        new Promise<FakeMaskPainter>((resolve) => {
          resolvePainter = resolve
        }),
    )

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    overlay.setMaskCanvasRef('node-1', canvas)
    await Promise.resolve()
    await nextTick()

    const total = INPAINT_MAX_PENDING_STROKES_PER_NODE + 4
    for (let index = 0; index < total; index += 1) {
      overlay.onCanvasPointerDown(pointerDown({ clientX: 60 + index * 10 }))
      window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, pointerType: 'mouse' }))
    }

    const latePainter = new FakeMaskPainter()
    painters.push(latePainter)
    resolvePainter(latePainter)
    await Promise.resolve()
    await nextTick()

    // Unbounded buffering would flush every drag; the cap keeps the newest.
    expect(latePainter.strokes).toHaveLength(INPAINT_MAX_PENDING_STROKES_PER_NODE)
    expect(latePainter.strokes[0]?.points?.[0]).toEqual({ x: 60 + 4 * 10, y: 45 })
  })

  it('decimates sub-threshold jitter from the buffer and the live preview', async () => {
    const { editorMode, overlay, painters } = createHarness()
    const canvas = createMaskCanvas()
    canvas.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    await mountPainter(overlay, canvas)
    overlay.onCanvasPointerDown(pointerDown())
    // Jitter under the 2px floor is dropped; the far sample is kept.
    window.dispatchEvent(
      new PointerEvent('pointermove', { clientX: 60.5, clientY: 45.2, pointerId: 1 }),
    )
    window.dispatchEvent(
      new PointerEvent('pointermove', { clientX: 61, clientY: 45, pointerId: 1 }),
    )
    window.dispatchEvent(
      new PointerEvent('pointermove', { clientX: 70, clientY: 50, pointerId: 1 }),
    )
    window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, pointerType: 'mouse' }))

    const painter = painters.at(-1)
    expect(painter?.appendedBatches).toEqual([[{ x: 60, y: 45 }], [{ x: 70, y: 50 }]])
    expect(painter?.endCalls).toBe(1)
  })

  it('caps buffered points per drag instead of growing without bound', async () => {
    const { editorMode, overlay, painters, createMaskPainter } = createHarness()
    const canvas = createMaskCanvas()
    canvas.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect

    let resolvePainter!: (painter: FakeMaskPainter) => void
    createMaskPainter.mockImplementationOnce(
      () =>
        new Promise<FakeMaskPainter>((resolve) => {
          resolvePainter = resolve
        }),
    )

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    overlay.setMaskCanvasRef('node-1', canvas)
    await Promise.resolve()
    await nextTick()

    // The painter is still loading: the drag only buffers, no live session.
    overlay.onCanvasPointerDown(pointerDown())
    for (let index = 1; index <= 2500; index += 1) {
      window.dispatchEvent(
        new PointerEvent('pointermove', {
          clientX: 60 + index * 5,
          clientY: 45,
          pointerId: 1,
          pointerType: 'mouse',
        }),
      )
    }
    window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, pointerType: 'mouse' }))

    const latePainter = new FakeMaskPainter()
    painters.push(latePainter)
    resolvePainter(latePainter)
    await Promise.resolve()
    await nextTick()

    expect(latePainter.strokes).toHaveLength(1)
    const buffered = latePainter.strokes[0]?.points ?? []
    expect(buffered.length).toBeLessThanOrEqual(INPAINT_MAX_POINTS_PER_STROKE)
    // Thinning preserves the shape instead of truncating the tail.
    expect(buffered.length).toBeGreaterThan(1000)
    expect(buffered.at(-1)).toEqual({ x: 60 + 2500 * 5, y: 45 })
  })

  it('disposes a stale painter creation when the element is reused', async () => {
    const { editorMode, overlay, painters, createMaskPainter } = createHarness()
    const canvas = createMaskCanvas()
    canvas.getBoundingClientRect = () => ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect

    const deferred: Array<{ painter: FakeMaskPainter; resolve: (p: FakeMaskPainter) => void }> = []
    createMaskPainter.mockImplementation(
      () =>
        new Promise<FakeMaskPainter>((resolve) => {
          const painter = new FakeMaskPainter()
          painters.push(painter)
          deferred.push({ painter, resolve })
        }),
    )

    overlay.setMaskCanvasRef('node-1', canvas)
    overlay.setMaskCanvasRef('node-1', null)
    overlay.setMaskCanvasRef('node-1', canvas)
    expect(deferred).toHaveLength(2)
    const staleCreation = deferred[0]
    const currentCreation = deferred[1]
    expect(staleCreation).toBeDefined()
    expect(currentCreation).toBeDefined()
    if (!staleCreation || !currentCreation) return

    // The stale creation resolves last: it must be disposed, never attached.
    currentCreation.resolve(currentCreation.painter)
    staleCreation.resolve(staleCreation.painter)
    await Promise.resolve()
    await Promise.resolve()
    await nextTick()

    const stale = staleCreation.painter
    const current = currentCreation.painter
    expect(stale.disposed).toBe(true)
    expect(current.disposed).toBe(false)

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    overlay.onCanvasPointerDown(pointerDown())
    window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, pointerType: 'mouse' }))

    expect(current?.liveBegins).toHaveLength(1)
    expect(stale.liveBegins).toHaveLength(0)
  })

  it('drops a stale committed preview when the data is cleared mid-load', async () => {
    const { editorMode, overlay, createMaskPainter } = createHarness()
    const target = createPixelCanvas(200, 100)
    target.canvas.getBoundingClientRect = () =>
      ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    overlay.setMaskCanvasRef('node-1', target.canvas)
    await Promise.resolve()
    await nextTick()
    overlay.onCanvasPointerDown(pointerDown())
    window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, pointerType: 'mouse' }))
    expect(overlay.commitMaskDraft()).toBe(true)

    // Remount with a still-loading painter, then re-enter so the committed
    // preview starts as a pixel load (no painter to baseline through).
    overlay.setMaskCanvasRef('node-1', null)
    createMaskPainter.mockImplementationOnce(() => new Promise<FakeMaskPainter>(() => {}))
    overlay.setMaskCanvasRef('node-1', target.canvas)
    await Promise.resolve()
    await nextTick()

    const images: Array<{ src: string; onload: (() => void) | null }> = []
    vi.stubGlobal(
      'Image',
      class {
        src = ''
        onload: (() => void) | null = null
        constructor() {
          images.push(this)
        }
      },
    )
    const createElementSpy = stubCanvasElementCreation()
    try {
      editorMode.exitRetouchMode()
      // Flush the null transition first: a synchronous exit+enter in one
      // tick collapses the watcher (old and new are both 'node-1'), so the
      // re-enter draw would never start.
      await nextTick()
      expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
      await nextTick()

      const image = images.at(-1)
      expect(image?.src).toBe('data:image/png;fake-preview')

      // The bitmap resolves after the discard: the draw must not happen.
      // The replacement painter stays loading so only the generation guard
      // can stop the stale load.
      createMaskPainter.mockImplementationOnce(() => new Promise<FakeMaskPainter>(() => {}))
      overlay.clearCommittedMaskData('node-1')
      target.context.putImageData.mockClear()
      image?.onload?.()

      expect(target.context.putImageData).not.toHaveBeenCalled()
    } finally {
      createElementSpy.mockRestore()
      vi.unstubAllGlobals()
    }
  })

  it('draws a fresh committed preview when nothing invalidates the load', async () => {
    const { editorMode, overlay, createMaskPainter } = createHarness()
    const target = createPixelCanvas(200, 100)
    target.canvas.getBoundingClientRect = () =>
      ({ left: 10, top: 20, width: 100, height: 50 }) as DOMRect

    expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
    overlay.setMaskCanvasRef('node-1', target.canvas)
    await Promise.resolve()
    await nextTick()
    overlay.onCanvasPointerDown(pointerDown())
    window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, pointerType: 'mouse' }))
    expect(overlay.commitMaskDraft()).toBe(true)

    overlay.setMaskCanvasRef('node-1', null)
    createMaskPainter.mockImplementationOnce(() => new Promise<FakeMaskPainter>(() => {}))
    overlay.setMaskCanvasRef('node-1', target.canvas)
    await Promise.resolve()
    await nextTick()

    const images: Array<{ src: string; onload: (() => void) | null }> = []
    vi.stubGlobal(
      'Image',
      class {
        src = ''
        onload: (() => void) | null = null
        constructor() {
          images.push(this)
        }
      },
    )
    const createElementSpy = stubCanvasElementCreation()
    try {
      editorMode.exitRetouchMode()
      // Flush the null transition first (see the stale-load spec above).
      await nextTick()
      expect(editorMode.enterRetouchMode('node-1', 'inpaint')).toBe(true)
      await nextTick()

      const image = images.at(-1)
      expect(image?.src).toBe('data:image/png;fake-preview')
      image?.onload?.()

      expect(target.context.putImageData).toHaveBeenCalled()
    } finally {
      createElementSpy.mockRestore()
      vi.unstubAllGlobals()
    }
  })
})
