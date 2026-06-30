type CanvasPerfMetadata = Record<string, string | number | boolean | null | undefined>

type CanvasPerfMetadataInput = CanvasPerfMetadata | (() => CanvasPerfMetadata)

const DEFAULT_MIN_DURATION_MS = 16

function isCanvasPerfProbeEnabled(): boolean {
  return import.meta.env.DEV && import.meta.env.VITE_CANVAS_PERF_PROBE === 'true'
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
  if (!isCanvasPerfProbeEnabled()) return task()
  const startedAt = now()
  try {
    return task()
  } finally {
    logCanvasPerf(label, now() - startedAt, metadata, minDurationMs)
  }
}

export async function measureCanvasPerfAsync<T>(
  label: string,
  metadata: CanvasPerfMetadataInput | undefined,
  task: () => Promise<T>,
  minDurationMs?: number,
): Promise<T> {
  if (!isCanvasPerfProbeEnabled()) return task()
  const startedAt = now()
  try {
    return await task()
  } finally {
    logCanvasPerf(label, now() - startedAt, metadata, minDurationMs)
  }
}
