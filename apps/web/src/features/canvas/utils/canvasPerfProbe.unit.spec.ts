import { afterEach, describe, expect, it, vi } from 'vitest'

describe('canvasPerfProbe', () => {
  afterEach(() => {
    delete window.__canvasInteractionBenchmarkCounters
    vi.restoreAllMocks()
    vi.unstubAllEnvs()
    vi.resetModules()
  })

  it('does not read the clock when neither the development probe nor benchmark counters are active', async () => {
    vi.stubEnv('VITE_CANVAS_PERF_PROBE', 'false')
    const now = vi.spyOn(performance, 'now')
    const { measureCanvasPerf } = await import('./canvasPerfProbe')

    expect(measureCanvasPerf('test.metric', undefined, () => 'done')).toBe('done')
    expect(now).not.toHaveBeenCalled()
  })

  it('aggregates a benchmark timing without enabling development console logging', async () => {
    vi.stubEnv('VITE_CANVAS_PERF_PROBE', 'false')
    window.__canvasInteractionBenchmarkCounters = {}
    vi.spyOn(performance, 'now').mockReturnValueOnce(10).mockReturnValueOnce(16.5)
    const info = vi.spyOn(console, 'info').mockImplementation(() => {})
    const { measureCanvasPerf } = await import('./canvasPerfProbe')

    expect(measureCanvasPerf('graph-store.rebuild', undefined, () => 'done')).toBe('done')
    expect(window.__canvasInteractionBenchmarkCounters.runtimeTimingMs).toEqual({
      'graph-store.rebuild': { count: 1, totalMs: 6.5, maxMs: 6.5 },
    })
    expect(info).not.toHaveBeenCalled()
  })
})
