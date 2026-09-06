import { isCanvasPerfProbeEnabled } from './canvasPerfProbe'

export type CanvasFrameProbeScene = 'pan' | 'zoom' | 'node-drag' | 'load-hydrate' | (string & {})

export type CanvasFrameProbeMetadata = Record<string, string | number | boolean | null | undefined>

export type CanvasFrameProbeMetadataInput =
  CanvasFrameProbeMetadata | (() => CanvasFrameProbeMetadata)

export type CanvasFrameProbeMark = {
  name: string
  atMs: number
  metadata?: CanvasFrameProbeMetadata
}

export type CanvasFrameProbeSummary = {
  scene: CanvasFrameProbeScene
  durationMs: number
  avgFrameMs: number
  p95FrameMs: number
  maxFrameMs: number
  droppedFrames: number
  sampleCount: number
  frameBudgetMs: number
  metadata?: CanvasFrameProbeMetadata
  marks: CanvasFrameProbeMark[]
}

export type CanvasFrameProbeSession = {
  readonly scene: CanvasFrameProbeScene
  mark(name: string, metadata?: CanvasFrameProbeMetadataInput): void
  end(metadata?: CanvasFrameProbeMetadataInput): CanvasFrameProbeSummary | undefined
  cleanup(): void
}

const FRAME_BUDGET_MS = 1000 / 60

type AnimationFrameApis = {
  request: (callback: FrameRequestCallback) => number
  cancel?: (handle: number) => void
}

function now(): number {
  return typeof performance !== 'undefined' ? performance.now() : Date.now()
}

function roundMs(value: number): number {
  return Math.round(value * 10) / 10
}

function resolveMetadata(
  metadata?: CanvasFrameProbeMetadataInput,
): CanvasFrameProbeMetadata | undefined {
  if (!metadata) return undefined
  return typeof metadata === 'function' ? metadata() : metadata
}

function mergeMetadata(
  startMetadata?: CanvasFrameProbeMetadata,
  endMetadata?: CanvasFrameProbeMetadataInput,
): CanvasFrameProbeMetadata | undefined {
  const resolvedEndMetadata = resolveMetadata(endMetadata)
  if (startMetadata && resolvedEndMetadata) return { ...startMetadata, ...resolvedEndMetadata }
  return resolvedEndMetadata ?? startMetadata
}

function getAnimationFrameApis(): AnimationFrameApis | undefined {
  if (typeof globalThis.requestAnimationFrame !== 'function') return undefined
  const request = globalThis.requestAnimationFrame.bind(globalThis)
  const apis: AnimationFrameApis = { request }
  if (typeof globalThis.cancelAnimationFrame === 'function') {
    apis.cancel = globalThis.cancelAnimationFrame.bind(globalThis)
  }
  return apis
}

function createInertCanvasFrameProbeSession(scene: CanvasFrameProbeScene): CanvasFrameProbeSession {
  return {
    scene,
    mark() {},
    end() {
      return undefined
    },
    cleanup() {},
  }
}

function percentile(sortedValues: number[], percentileValue: number): number {
  if (sortedValues.length === 0) return 0
  const index = Math.ceil(sortedValues.length * percentileValue) - 1
  return sortedValues[Math.max(0, Math.min(index, sortedValues.length - 1))] ?? 0
}

function countDroppedFrames(frameDeltas: number[]): number {
  return frameDeltas.reduce((total, deltaMs) => {
    const expectedFrames = Math.max(1, Math.round(deltaMs / FRAME_BUDGET_MS))
    return total + expectedFrames - 1
  }, 0)
}

function summarizeFrameProbe(
  scene: CanvasFrameProbeScene,
  durationMs: number,
  frameDeltas: number[],
  metadata: CanvasFrameProbeMetadata | undefined,
  marks: CanvasFrameProbeMark[],
): CanvasFrameProbeSummary {
  const sortedFrameDeltas = [...frameDeltas].sort((left, right) => left - right)
  const sampleCount = frameDeltas.length
  const totalFrameMs = frameDeltas.reduce((total, deltaMs) => total + deltaMs, 0)
  const summary: CanvasFrameProbeSummary = {
    scene,
    durationMs: roundMs(durationMs),
    avgFrameMs: sampleCount > 0 ? roundMs(totalFrameMs / sampleCount) : 0,
    p95FrameMs: roundMs(percentile(sortedFrameDeltas, 0.95)),
    maxFrameMs: roundMs(sortedFrameDeltas.at(-1) ?? 0),
    droppedFrames: countDroppedFrames(frameDeltas),
    sampleCount,
    frameBudgetMs: roundMs(FRAME_BUDGET_MS),
    marks: [...marks],
  }
  if (metadata) summary.metadata = metadata
  return summary
}

export function isCanvasFrameProbeEnabled(): boolean {
  return isCanvasPerfProbeEnabled() && getAnimationFrameApis() !== undefined
}

export function beginCanvasFrameProbe(
  scene: CanvasFrameProbeScene,
  metadata?: CanvasFrameProbeMetadataInput,
): CanvasFrameProbeSession {
  if (!isCanvasPerfProbeEnabled()) return createInertCanvasFrameProbeSession(scene)

  const animationFrame = getAnimationFrameApis()
  if (!animationFrame) return createInertCanvasFrameProbeSession(scene)

  const startedAt = now()
  const startMetadata = resolveMetadata(metadata)
  const frameDeltas: number[] = []
  const marks: CanvasFrameProbeMark[] = []
  let isActive = true
  let lastFrameAt: number | undefined
  let frameId: number | undefined

  const stop = () => {
    if (!isActive) return false
    isActive = false
    if (frameId !== undefined) {
      animationFrame.cancel?.(frameId)
      frameId = undefined
    }
    return true
  }

  const scheduleNextFrame = () => {
    frameId = animationFrame.request(onFrame)
  }

  const onFrame: FrameRequestCallback = (timestamp) => {
    if (!isActive) return
    if (lastFrameAt !== undefined) {
      const deltaMs = timestamp - lastFrameAt
      if (Number.isFinite(deltaMs) && deltaMs >= 0) {
        frameDeltas.push(deltaMs)
      }
    }
    lastFrameAt = timestamp
    scheduleNextFrame()
  }

  scheduleNextFrame()

  return {
    scene,
    mark(name, markMetadata) {
      if (!isActive) return
      const resolvedMetadata = resolveMetadata(markMetadata)
      const mark: CanvasFrameProbeMark = {
        name,
        atMs: roundMs(now() - startedAt),
      }
      if (resolvedMetadata) mark.metadata = resolvedMetadata
      marks.push(mark)
    },
    end(endMetadata) {
      if (!stop()) return undefined
      const summary = summarizeFrameProbe(
        scene,
        now() - startedAt,
        frameDeltas,
        mergeMetadata(startMetadata, endMetadata),
        marks,
      )
      console.info(`[canvas:perf] frame ${scene} ${summary.durationMs}ms`, summary)
      return summary
    },
    cleanup() {
      stop()
    },
  }
}
