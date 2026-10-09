/**
 * Reserved world-capture-to-canvas reflow type contract.
 *
 * A future 3D world capture producer will implement `WorldCaptureToCanvas`
 * and hand its image result back into the Canvas as ordinary image input.
 * No runtime implementation or consumer is attached until that producer exists.
 *
 * Contract notes for the future producer (not implemented here):
 *
 * - This module is domain-pure: zero 3D dependency, zero Fabric dependency,
 *   zero Vue reactivity. The producer adapter must live outside this module
 *   and translate its native result into `WorldCaptureIntent` / data URL.
 * - Reflow enters the Canvas only through the existing graph-editing intents
 *   (creation / media-input mutation), never by writing Vue Flow projections
 *   or `CanvasGraphStore` internals directly.
 * - The graphics layer (Fabric: things on the canvas) and the orchestration
 *   layer (Vue Flow: flow between things) stay split: a capture result is
 *   image content for a Canvas Node, not a graph mutation by itself.
 */

/** Request to flow one world capture back into the Canvas. */
export type WorldCaptureIntent = {
  /** Opaque producer-side handle identifying the capture source. */
  readonly sourceId: string
  /** Optional target Canvas Node id; absent means create a new image node. */
  readonly targetNodeId?: string
}

/** Captured image payload, or `null` when no capture is available. */
export type WorldCaptureResult = {
  /** Image bytes for the reflow path (same role as an upload data URL). */
  readonly imageDataUrl: string
} | null

export type WorldCaptureToCanvas = {
  /** Whether a live 3D capture producer is attached. */
  readonly isAvailable: boolean
  /** Captures image content for reflow, or returns `null` when unavailable. */
  captureToCanvas(intent: WorldCaptureIntent): WorldCaptureResult
}
