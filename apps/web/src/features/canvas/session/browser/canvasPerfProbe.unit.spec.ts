import { afterEach, describe, expect, it, test, vi } from 'vitest'

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
      'graph-store.rebuild': {
        count: 1,
        totalMs: 6.5,
        maxMs: 6.5,
        sampleCount: 1,
        p50Ms: 6.5,
        p95Ms: 6.5,
      },
    })
    expect(info).not.toHaveBeenCalled()
  })

  test.each(['replace', 'delete-reinstall', 'enable-later'])(
    'keeps deferred save timings out of a new collection after %s',
    async (reset) => {
      vi.stubEnv('VITE_CANVAS_PERF_PROBE', 'false')
      if (reset !== 'enable-later') window.__canvasInteractionBenchmarkCounters = {}
      const { measureCanvasPerfAsync } = await import('./canvasPerfProbe')
      let finishSave!: () => void
      const saving = measureCanvasPerfAsync(
        'board.save.session',
        undefined,
        () =>
          new Promise<void>((resolve) => {
            finishSave = resolve
          }),
      )
      if (reset === 'delete-reinstall') delete window.__canvasInteractionBenchmarkCounters
      const replacement = (window.__canvasInteractionBenchmarkCounters = {})
      finishSave()
      await saving
      expect(replacement).toEqual({})

      await measureCanvasPerfAsync('board.save.session', undefined, async () => 'saved')
      expect(
        window.__canvasInteractionBenchmarkCounters.runtimeTimingMs?.['board.save.session']?.count,
      ).toBe(1)
    },
  )

  test('ignores a timing if a synchronous task resets its collection', async () => {
    vi.stubEnv('VITE_CANVAS_PERF_PROBE', 'false')
    window.__canvasInteractionBenchmarkCounters = {}
    const { measureCanvasPerf } = await import('./canvasPerfProbe')
    measureCanvasPerf('graph-store.rebuild', undefined, () => {
      window.__canvasInteractionBenchmarkCounters = {}
    })
    expect(window.__canvasInteractionBenchmarkCounters).toEqual({})
  })
})
