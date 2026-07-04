export type CanvasInteractionBenchmarkCounters = Record<string, number> & {
  graphCommands?: Record<string, number>
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
