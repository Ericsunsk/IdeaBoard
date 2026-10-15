type CanvasInteractionTiming = {
  count: number
  totalMs: number
  maxMs: number
  sampleCount: number
  p50Ms: number
  p95Ms: number
}

export type CanvasInteractionBenchmarkCounters = {
  [name: string]:
    number | Record<string, number> | Record<string, CanvasInteractionTiming> | undefined
  gauges?: Record<string, number>
  graphCommands?: Record<string, number>
  runtimeTimingMs?: Record<string, CanvasInteractionTiming>
}

const TIMING_SAMPLE_LIMIT = 256
const timingSamples = new WeakMap<CanvasInteractionTiming, number[]>()

declare global {
  interface Window {
    __canvasInteractionBenchmarkCounters?: CanvasInteractionBenchmarkCounters
  }
}

function readCounters(): CanvasInteractionBenchmarkCounters | null {
  if (typeof window === 'undefined') return null
  return window.__canvasInteractionBenchmarkCounters ?? null
}

export function recordCanvasInteractionCount(name: string, amount = 1) {
  const counters = readCounters()
  if (!counters || !Number.isFinite(amount)) return
  const current = counters[name]
  if (current !== undefined && typeof current !== 'number') return
  counters[name] = (current ?? 0) + amount
}

export function recordCanvasGraphCommand(command: string) {
  const counters = readCounters()
  if (!counters) return
  recordCanvasInteractionCount('graphCommandCount')
  const graphCommands = counters.graphCommands ?? {}
  graphCommands[command] = (graphCommands[command] ?? 0) + 1
  counters.graphCommands = graphCommands
}

export function isCanvasInteractionBenchmarkActive(): boolean {
  return readCounters() !== null
}

export function beginCanvasInteractionTiming(name: string) {
  const counters = readCounters()
  if (!counters) return null
  const startedAt = performance.now()
  return (options?: { count?: string; shouldRecordTiming?: boolean }) => {
    if (readCounters() !== counters) return
    if (options?.count) recordCanvasInteractionCount(options.count)
    if (options?.shouldRecordTiming !== false) {
      recordTiming(counters, name, performance.now() - startedAt)
    }
  }
}

function recordTiming(
  counters: CanvasInteractionBenchmarkCounters,
  name: string,
  durationMs: number,
) {
  if (!Number.isFinite(durationMs) || durationMs < 0) return

  const timings = counters.runtimeTimingMs ?? {}
  const current = timings[name] ?? {
    count: 0,
    totalMs: 0,
    maxMs: 0,
    sampleCount: 0,
    p50Ms: 0,
    p95Ms: 0,
  }
  const samples = timingSamples.get(current) ?? []
  samples[current.count % TIMING_SAMPLE_LIMIT] = durationMs
  timingSamples.set(current, samples)
  const sorted = [...samples].sort((left, right) => left - right)
  current.count += 1
  current.totalMs += durationMs
  current.maxMs = Math.max(current.maxMs, durationMs)
  current.sampleCount = samples.length
  current.p50Ms = sorted[Math.ceil(sorted.length * 0.5) - 1] ?? 0
  current.p95Ms = sorted[Math.ceil(sorted.length * 0.95) - 1] ?? 0
  timings[name] = current
  counters.runtimeTimingMs = timings
}

export function recordCanvasInteractionGauge(name: string, value: number) {
  const counters = readCounters()
  if (!counters || !Number.isFinite(value) || value < 0) return
  const gauges = counters.gauges ?? {}
  gauges[name] = value
  counters.gauges = gauges
}
