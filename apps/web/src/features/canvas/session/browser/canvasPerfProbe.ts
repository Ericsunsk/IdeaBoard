import { beginCanvasInteractionTiming } from './canvasInteractionMetrics'

type CanvasPerfMetadata = Record<string, string | number | boolean | null | undefined>

type CanvasPerfMetadataInput = CanvasPerfMetadata | (() => CanvasPerfMetadata)

const DEFAULT_MIN_DURATION_MS = 16

export function isCanvasPerfProbeEnabled(): boolean {
  const env = import.meta.env
  return env?.DEV === true && env.VITE_CANVAS_PERF_PROBE === 'true'
}

function now(): number {
  return typeof performance !== 'undefined' ? performance.now() : Date.now()
}

function resolveMetadata(metadata?: CanvasPerfMetadataInput): CanvasPerfMetadata | undefined {
  if (!metadata) return undefined
  return typeof metadata === 'function' ? metadata() : metadata
}

function logCanvasPerf(
  label: string,
  durationMs: number,
  metadata?: CanvasPerfMetadataInput,
  minDurationMs = DEFAULT_MIN_DURATION_MS,
) {
  if (!isCanvasPerfProbeEnabled() || durationMs < minDurationMs) return
  const roundedMs = Math.round(durationMs * 10) / 10
  const resolvedMetadata = resolveMetadata(metadata)
  if (resolvedMetadata) {
    console.info(`[canvas:perf] ${label} ${roundedMs}ms`, resolvedMetadata)
    return
  }
  console.info(`[canvas:perf] ${label} ${roundedMs}ms`)
}

export function measureCanvasPerf<T>(
  label: string,
  metadata: CanvasPerfMetadataInput | undefined,
  task: () => T,
  minDurationMs?: number,
): T {
  const isPerfProbeEnabled = isCanvasPerfProbeEnabled()
  const finishTiming = beginCanvasInteractionTiming(label)
  if (!isPerfProbeEnabled && !finishTiming) return task()
  const startedAt = isPerfProbeEnabled ? now() : 0
  try {
    return task()
  } finally {
    const durationMs = isPerfProbeEnabled ? now() - startedAt : 0
    finishTiming?.()
    if (isPerfProbeEnabled) logCanvasPerf(label, durationMs, metadata, minDurationMs)
  }
}

export async function measureCanvasPerfAsync<T>(
  label: string,
  metadata: CanvasPerfMetadataInput | undefined,
  task: () => Promise<T>,
  minDurationMs?: number,
): Promise<T> {
  const isPerfProbeEnabled = isCanvasPerfProbeEnabled()
  const finishTiming = beginCanvasInteractionTiming(label)
  if (!isPerfProbeEnabled && !finishTiming) return task()
  const startedAt = isPerfProbeEnabled ? now() : 0
  try {
    return await task()
  } finally {
    const durationMs = isPerfProbeEnabled ? now() - startedAt : 0
    finishTiming?.()
    if (isPerfProbeEnabled) logCanvasPerf(label, durationMs, metadata, minDurationMs)
  }
}
