import { Canvas, FabricImage, PencilBrush, Point } from 'fabric'
import { clamp } from '../../../../../shared/utils/number.ts'
import { buildMaskCommitResult } from '../lib/maskPaintingCanvas.ts'
import {
  clampMaskBrushSize,
  INPAINT_DEFAULT_BRUSH_SIZE,
  INPAINT_HISTORY_LIMIT,
  MASK_PAINTING_COLOR,
  MASK_PAINTING_EMPTY_ALPHA_THRESHOLD,
} from '../config/constants.ts'
import type { FabricObject } from 'fabric'
import type {
  MaskPainter,
  MaskPainterClientPoint,
  MaskPainterCommit,
  MaskPainterDrawMode,
  MaskPainterOptions,
  MaskPainterViewport,
} from './painter.ts'

/**
 * Injectable Fabric constructors. Production defaults use the tree-shaken
 * named imports above (`Canvas`, `PencilBrush`, `Point`, `FabricImage` only:
 * paint/erase need nothing else; `EraserBrush` is not in Fabric core).
 * Specs inject fakes so happy-dom never constructs a real Fabric canvas.
 */
export type FabricMaskPainterDependencies = {
  createCanvas: (element: HTMLCanvasElement, options: FabricMaskCanvasOptions) => Canvas
  createPaintBrush: (canvas: Canvas) => PencilBrush
  createEraseBrush: (canvas: Canvas) => PencilBrush
  loadBaselineImage?: (dataUrl: string) => Promise<FabricBaselineImage>
}

/**
 * Structural baseline image: the committed-preview bitmap re-attached as the
 * Fabric background so later renders preserve it. Structural (not
 * `FabricImage`) so specs can inject stubs without image loading.
 */
export type FabricBaselineImage = {
  width?: number
  height?: number
  set: (options: { scaleX?: number; scaleY?: number }) => unknown
}

export type FabricMaskCanvasOptions = {
  width: number
  height: number
  enableRetinaScaling: boolean
  selection: boolean
}

type StrokeHistoryEntry =
  | { kind: 'stroke'; objects: FabricObject[] }
  | { kind: 'clear'; objects: FabricObject[]; background: FabricObject | undefined }

type LiveStrokeSession = {
  brush: PencilBrush
  erase: boolean
  viewport: MaskPainterViewport
  objectCount: number
  hasPoints: boolean
}

type CanvasPlanePoint = {
  x: number
  y: number
}

// Fabric checks `isPrimary` to accept a pointer event; programmatic strokes
// drive brushes directly, so a minimal primary stub stands in for the event.
const PRIMARY_POINTER_STUB = { isPrimary: true } as PointerEvent

const ERASE_BRUSH_COLOR = '#000000'

function readDevicePixelRatio(): number {
  if (typeof window === 'undefined') return 1
  const ratio = Number(window.devicePixelRatio)
  return Number.isFinite(ratio) && ratio > 0 ? ratio : 1
}

const defaultDependencies: FabricMaskPainterDependencies = {
  createCanvas: (element, options) => new Canvas(element, options),
  createPaintBrush: (canvas) => new PencilBrush(canvas),
  createEraseBrush: (canvas) => new PencilBrush(canvas),
  loadBaselineImage: (dataUrl) => FabricImage.fromURL(dataUrl),
}

function toCssSize(value: number): number {
  return Math.max(1, Math.round(value) || 0)
}

function clampToPlane(value: number, max: number): number {
  return clamp(value, 0, max)
}

export class FabricMaskPainter implements MaskPainter {
  private readonly host: HTMLCanvasElement
  private readonly canvas: Canvas
  private readonly dependencies: FabricMaskPainterDependencies
  private readonly paintBrush: PencilBrush
  private readonly eraseBrush: PencilBrush
  private readonly planeWidth: number
  private readonly planeHeight: number
  private readonly undoStack: StrokeHistoryEntry[] = []
  private readonly redoStack: StrokeHistoryEntry[] = []
  private live: LiveStrokeSession | null = null
  private baselineImage: FabricObject | undefined = undefined
  private baselineToken = 0
  private brushSize: number
  private disposed = false

  constructor(
    host: HTMLCanvasElement,
    options: MaskPainterOptions,
    dependencies: FabricMaskPainterDependencies = defaultDependencies,
  ) {
    this.host = host
    this.dependencies = dependencies
    this.planeWidth = toCssSize(options.width)
    this.planeHeight = toCssSize(options.height)
    this.brushSize = clampMaskBrushSize(options.brushSize ?? INPAINT_DEFAULT_BRUSH_SIZE)
    // DPR alignment is Fabric-owned: `enableRetinaScaling` sizes the backing
    // store (plane x devicePixelRatio) while brushes keep drawing in CSS-px
    // canvas-plane coordinates at zoom 1 with an identity transform.
    this.canvas = dependencies.createCanvas(host, {
      width: this.planeWidth,
      height: this.planeHeight,
      enableRetinaScaling: true,
      selection: false,
    })
    this.paintBrush = dependencies.createPaintBrush(this.canvas)
    this.eraseBrush = dependencies.createEraseBrush(this.canvas)
    this.paintBrush.color = MASK_PAINTING_COLOR
    this.paintBrush.width = this.brushSize
    this.eraseBrush.color = ERASE_BRUSH_COLOR
    this.eraseBrush.width = this.brushSize
  }

  paintStroke(points: readonly MaskPainterClientPoint[], viewport: MaskPainterViewport): void {
    if (this.disposed) return
    const planePoints = this.projectStroke(points, viewport)
    if (!planePoints) return
    this.healBackingStore()
    const objectCount = this.canvas.getObjects().length
    this.driveBrush(this.paintBrush, planePoints)
    this.finishStrokeObjects(objectCount, false)
  }

  eraseStroke(points: readonly MaskPainterClientPoint[], viewport: MaskPainterViewport): void {
    if (this.disposed) return
    const planePoints = this.projectStroke(points, viewport)
    if (!planePoints) return
    this.healBackingStore()
    const objectCount = this.canvas.getObjects().length
    this.driveBrush(this.eraseBrush, planePoints)
    this.finishStrokeObjects(objectCount, true)
  }

  beginStroke(mode: MaskPainterDrawMode, viewport: MaskPainterViewport): void {
    if (this.disposed) return
    if (!viewport.width || !viewport.height) return
    // A live session must never overlap another: finalize the previous one
    // so its objects stay undoable instead of leaking into the next entry.
    if (this.live) this.endStroke()
    if (this.disposed) return
    this.healBackingStore()
    const erase = mode === 'erase'
    this.live = {
      brush: erase ? this.eraseBrush : this.paintBrush,
      erase,
      viewport: { ...viewport },
      objectCount: this.canvas.getObjects().length,
      hasPoints: false,
    }
  }

  appendStrokePoints(points: readonly MaskPainterClientPoint[]): void {
    if (this.disposed) return
    const session = this.live
    if (!session) return
    const planePoints = this.projectPoints(points, session.viewport)
    if (!planePoints) return
    // Incremental by design: each sample drives one brush move on the Fabric
    // top canvas. The full stroke is never replayed per move, so long drags
    // stay O(appended) instead of O(moves^2).
    for (const point of planePoints) {
      const target = new Point(point.x, point.y)
      if (!session.hasPoints) {
        session.brush.onMouseDown(target, { e: PRIMARY_POINTER_STUB })
        session.hasPoints = true
      } else {
        session.brush.onMouseMove(target, { e: PRIMARY_POINTER_STUB })
      }
    }
  }

  endStroke(): void {
    const session = this.live
    this.live = null
    if (this.disposed || !session) return
    if (!session.hasPoints) return
    session.brush.onMouseUp({ e: PRIMARY_POINTER_STUB })
    this.finishStrokeObjects(session.objectCount, session.erase)
  }

  cancelStroke(): void {
    const session = this.live
    this.live = null
    if (this.disposed || !session) return
    if (!session.hasPoints) return
    // Finalize the partial preview first: Fabric clears the top canvas when
    // a path is committed, so the cancelled preview leaves no ghost behind.
    session.brush.onMouseUp({ e: PRIMARY_POINTER_STUB })
    const added = this.canvas.getObjects().slice(session.objectCount)
    if (added.length > 0) this.canvas.remove(...added)
    this.renderNow()
  }

  setBaselineImage(dataUrl: string | null): void {
    this.baselineToken += 1
    if (this.disposed) return
    if (!dataUrl) {
      this.baselineImage = undefined
      try {
        this.setBackground(undefined)
        this.renderNow()
      } catch {
        // Detached host: dropping the reference is enough.
      }
      return
    }
    const token = this.baselineToken
    const loader = this.dependencies.loadBaselineImage ?? defaultDependencies.loadBaselineImage
    if (!loader) return
    void Promise.resolve()
      .then(() => loader(dataUrl))
      .then((image) => {
        if (this.disposed || token !== this.baselineToken) return
        try {
          // The committed mask is a backing-store bitmap (plane x DPR at
          // commit time); scale it back onto the CSS plane so it renders 1:1
          // with live stroke coordinates instead of overflowing.
          const sourceWidth = image.width || 1
          const sourceHeight = image.height || 1
          image.set({
            scaleX: this.planeWidth / sourceWidth,
            scaleY: this.planeHeight / sourceHeight,
          })
          const background = image as unknown as FabricObject
          this.baselineImage = background
          this.setBackground(background)
          this.renderNow()
        } catch {
          // Detached host: the baseline reference is dropped with dispose.
        }
      })
      .catch(() => {
        // A failed baseline load leaves strokes committable; the preview
        // simply shows no committed layer until the next successful restore.
      })
  }

  setBrushSize(size: number): void {
    this.brushSize = clampMaskBrushSize(size)
    this.paintBrush.width = this.brushSize
    this.eraseBrush.width = this.brushSize
  }

  getBrushSize(): number {
    return this.brushSize
  }

  canUndo(): boolean {
    return this.undoStack.length > 0
  }

  canRedo(): boolean {
    return this.redoStack.length > 0
  }

  undo(): void {
    if (this.disposed) return
    // Undo never splits a live session: finalize it first so the undone
    // entry boundary stays exactly one stroke.
    if (this.live) this.endStroke()
    const entry = this.undoStack.pop()
    if (!entry) return
    if (entry.kind === 'clear') {
      // Stack discipline guarantees the canvas holds the post-clear state
      // here, so re-adding restores the exact pre-clear z-order.
      if (entry.objects.length > 0) this.canvas.add(...entry.objects)
      if (entry.background) {
        this.baselineImage = entry.background
        this.setBackground(entry.background)
      }
    } else if (entry.objects.length > 0) {
      this.canvas.remove(...entry.objects)
    }
    this.redoStack.push(entry)
    this.renderNow()
  }

  redo(): void {
    if (this.disposed) return
    if (this.live) this.endStroke()
    const entry = this.redoStack.pop()
    if (!entry) return
    if (entry.kind === 'clear') {
      if (entry.objects.length > 0) this.canvas.remove(...entry.objects)
      if (entry.background && this.canvas.backgroundImage === entry.background) {
        this.baselineImage = undefined
        this.setBackground(undefined)
      }
    } else if (entry.objects.length > 0) {
      this.canvas.add(...entry.objects)
    }
    this.undoStack.push(entry)
    this.renderNow()
  }

  clear(): void {
    if (this.disposed) return
    // In-flight baseline loads belong to the pre-clear state.
    this.baselineToken += 1
    if (this.live) this.endStroke()
    if (!this.hasPaint()) return
    const objects = this.canvas.getObjects().slice()
    const background = this.baselineImage
    if (objects.length > 0) this.canvas.remove(...objects)
    this.baselineImage = undefined
    this.setBackground(undefined)
    this.undoStack.push({ kind: 'clear', objects, background })
    if (this.undoStack.length > INPAINT_HISTORY_LIMIT) {
      this.undoStack.splice(0, this.undoStack.length - INPAINT_HISTORY_LIMIT)
    }
    this.redoStack.length = 0
    // Synchronous render: the pixel read in `hasPaint`/`commit` must observe
    // the object removal immediately, not after the next animation frame.
    this.renderNow()
  }

  hasPaint(): boolean {
    const context = this.host.getContext('2d', { willReadFrequently: true })
    if (!context || !this.host.width || !this.host.height) return false
    const data = context.getImageData(0, 0, this.host.width, this.host.height).data
    const threshold = MASK_PAINTING_EMPTY_ALPHA_THRESHOLD * 255
    for (let index = 3; index < data.length; index += 4) {
      if ((data[index] ?? 0) > threshold) return true
    }
    return false
  }

  commit(): MaskPainterCommit | null {
    if (this.disposed) return null
    // Dual-mask encoding stays single-sourced in `buildMaskCommitResult`:
    // execution is the opaque grayscale mask, preview is the tinted mask.
    // The Execution Plan consumer (`planImageInpaintAction`) is downstream-blind.
    return buildMaskCommitResult(this.host)
  }

  dispose(): void {
    if (this.disposed) return
    this.disposed = true
    this.live = null
    this.baselineToken += 1
    this.baselineImage = undefined
    this.undoStack.length = 0
    this.redoStack.length = 0
    this.canvas.dispose()
  }

  /**
   * `backgroundImage` is declared optional-without-undefined under
   * `exactOptionalPropertyTypes`; a local structural view keeps the
   * assignment type-safe without touching Fabric types.
   */
  private setBackground(image: FabricObject | undefined): void {
    const canvas = this.canvas as unknown as { backgroundImage?: FabricObject | undefined }
    canvas.backgroundImage = image
  }

  /**
   * Vue owns the width/height attrs (CSS plane size) while Fabric owns the
   * backing store (plane x DPR). A re-render that resets the backing to the
   * CSS size would stretch DPR pixels; re-applying Fabric retina dimensions
   * is object-safe (vectors + background re-render, no pixel snapshots).
   */
  private healBackingStore(): void {
    try {
      if (!(readDevicePixelRatio() > 1)) return
      if (this.host.width !== this.planeWidth || this.host.height !== this.planeHeight) return
      const resizable = this.canvas as unknown as {
        setDimensions?: (size: { width: number; height: number }) => void
      }
      resizable.setDimensions?.({ width: this.planeWidth, height: this.planeHeight })
    } catch {
      // Spec fakes and detached hosts: best effort only.
    }
  }

  private renderNow(): void {
    try {
      this.canvas.renderAll()
    } catch {
      this.canvas.requestRenderAll()
    }
  }

  private projectStroke(
    points: readonly MaskPainterClientPoint[],
    viewport: MaskPainterViewport,
  ): CanvasPlanePoint[] | null {
    const projected = this.projectPoints(points, viewport)
    if (!projected) return null
    if (projected.length === 1) {
      // A tap without movement still paints a dot; Fabric drops empty
      // single-point paths, so feed the point twice (its dot fallback).
      const only = projected[0]
      if (only) projected.push({ x: only.x, y: only.y })
    }
    return projected
  }

  private projectPoints(
    points: readonly MaskPainterClientPoint[],
    viewport: MaskPainterViewport,
  ): CanvasPlanePoint[] | null {
    if (points.length === 0) return null
    if (!viewport.width || !viewport.height) return null
    const scaleX = this.planeWidth / viewport.width
    const scaleY = this.planeHeight / viewport.height
    return points.map((point) => ({
      x: clampToPlane((point.x - viewport.left) * scaleX, this.planeWidth),
      y: clampToPlane((point.y - viewport.top) * scaleY, this.planeHeight),
    }))
  }

  private driveBrush(brush: PencilBrush, points: CanvasPlanePoint[]): void {
    const first = points[0]
    if (!first) return
    brush.onMouseDown(new Point(first.x, first.y), { e: PRIMARY_POINTER_STUB })
    for (const point of points.slice(1)) {
      brush.onMouseMove(new Point(point.x, point.y), { e: PRIMARY_POINTER_STUB })
    }
    brush.onMouseUp({ e: PRIMARY_POINTER_STUB })
  }

  private finishStrokeObjects(objectCount: number, erase: boolean): void {
    const added = this.canvas.getObjects().slice(objectCount)
    if (erase) {
      // Fabric core has no EraserBrush (it ships as an extension); erase by
      // compositing the freshly added paths with destination-out, mirroring
      // the legacy raster erase semantics.
      for (const object of added) {
        object.globalCompositeOperation = 'destination-out'
      }
    }
    if (added.length > 0) {
      this.undoStack.push({ kind: 'stroke', objects: added })
      if (this.undoStack.length > INPAINT_HISTORY_LIMIT) {
        this.undoStack.splice(0, this.undoStack.length - INPAINT_HISTORY_LIMIT)
      }
      this.redoStack.length = 0
    }
    this.renderNow()
  }
}

export function createFabricMaskPainter(
  host: HTMLCanvasElement,
  options: MaskPainterOptions,
  dependencies?: FabricMaskPainterDependencies,
): MaskPainter {
  return new FabricMaskPainter(host, options, dependencies)
}
