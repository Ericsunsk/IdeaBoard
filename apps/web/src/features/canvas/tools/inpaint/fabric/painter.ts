import type { ImageInpaintCommittedData, ImageMaskDrawMode } from '../model.ts'

/**
 * Narrow mask painter contract for the Fabric seam.
 *
 * This module is domain-pure: zero Fabric dependency, zero Vue reactivity.
 * Geometry (client to canvas-plane projection, clamping) and DPR alignment
 * live in the adapter (`adapter.ts`), never in callers.
 */

/** Viewport-space pointer sample (CSS pixels, e.g. `PointerEvent.clientX/Y`). */
export type MaskPainterClientPoint = {
  x: number
  y: number
}

/** Captured layout of the overlay box a stroke is projected against. */
export type MaskPainterViewport = {
  left: number
  top: number
  width: number
  height: number
}

export type MaskPainterDrawMode = ImageMaskDrawMode

/**
 * Dual mask payload. Field-for-field identical to the legacy
 * `ImageInpaintCommittedData`: `executionMaskDataUrl` feeds the Execution
 * Plan (`planImageInpaintAction`), `maskDataUrl` is the preview tint.
 */
export type MaskPainterCommit = ImageInpaintCommittedData

export type MaskPainterOptions = {
  /** Display width in CSS pixels. */
  width: number
  /** Display height in CSS pixels. */
  height: number
  brushSize?: number
}

export type MaskPainter = {
  /**
   * One-shot stroke (tap dot, buffered flush while the painter was loading).
   * Records exactly one history entry.
   */
  paintStroke(points: readonly MaskPainterClientPoint[], viewport: MaskPainterViewport): void
  eraseStroke(points: readonly MaskPainterClientPoint[], viewport: MaskPainterViewport): void
  /**
   * Live incremental session: exactly one history entry per begin to end.
   * The viewport is captured once at begin (no per-move re-read); per-move
   * appends render incrementally without replaying the whole stroke.
   * A session with zero appended points records nothing.
   */
  beginStroke(mode: MaskPainterDrawMode, viewport: MaskPainterViewport): void
  appendStrokePoints(points: readonly MaskPainterClientPoint[]): void
  endStroke(): void
  /** Discards a live session without recording history. */
  cancelStroke(): void
  /**
   * Committed-preview baseline. Implementations must keep it render-safe
   * (it survives the painter's own re-renders); `null` clears it.
   */
  setBaselineImage(dataUrl: string | null): void
  setBrushSize(size: number): void
  getBrushSize(): number
  canUndo(): boolean
  canRedo(): boolean
  undo(): void
  redo(): void
  clear(): void
  hasPaint(): boolean
  commit(): MaskPainterCommit | null
  dispose(): void
}
