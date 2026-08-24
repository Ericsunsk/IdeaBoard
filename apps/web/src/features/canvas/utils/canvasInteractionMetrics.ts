export type CanvasInteractionBenchmarkCounters = Record<string, number> & {
  graphCommands?: Record<string, number>
  runtimeTimingMs?: Record<
    string,
    {
      count: number
      totalMs: number
      maxMs: number
    }
  >
}

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
  if (!counters) return
  counters[name] = (counters[name] ?? 0) + amount
}

export function recordCanvasGraphCommand(command: string) {
  const counters = readCounters()
  if (!counters) return
  counters.graphCommandCount = (counters.graphCommandCount ?? 0) + 1
  const graphCommands = counters.graphCommands ?? {}
  graphCommands[command] = (graphCommands[command] ?? 0) + 1
  counters.graphCommands = graphCommands
}

export function isCanvasInteractionBenchmarkActive(): boolean {
  return readCounters() !== null
}

export function recordCanvasInteractionTiming(name: string, durationMs: number) {
  const counters = readCounters()
  if (!counters || !Number.isFinite(durationMs) || durationMs < 0) return

  const timings = counters.runtimeTimingMs ?? {}
  const current = timings[name] ?? { count: 0, totalMs: 0, maxMs: 0 }
  timings[name] = {
    count: current.count + 1,
    totalMs: current.totalMs + durationMs,
    maxMs: Math.max(current.maxMs, durationMs),
  }
  counters.runtimeTimingMs = timings
}
