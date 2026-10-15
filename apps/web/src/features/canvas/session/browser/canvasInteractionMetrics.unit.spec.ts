import { afterEach, describe, expect, test, vi } from 'vitest'
import {
  isCanvasInteractionBenchmarkActive,
  recordCanvasGraphCommand,
  recordCanvasInteractionCount,
  recordCanvasInteractionGauge,
  beginCanvasInteractionTiming,
} from './canvasInteractionMetrics'

afterEach(() => {
  delete window.__canvasInteractionBenchmarkCounters
  vi.restoreAllMocks()
})

describe('Canvas interaction metrics', () => {
  test('stays inert without an explicitly installed collection', () => {
    recordCanvasInteractionCount('nodes')
    recordCanvasGraphCommand('move')
    const now = vi.spyOn(performance, 'now')
    expect(beginCanvasInteractionTiming('save')).toBeNull()
    expect(now).not.toHaveBeenCalled()
    recordCanvasInteractionGauge('previewPixels', 1200)
    expect(isCanvasInteractionBenchmarkActive()).toBe(false)
    expect(window.__canvasInteractionBenchmarkCounters).toBeUndefined()
  })

  test('counts commands and signed resource changes without overwriting structured metrics', () => {
    const counters = (window.__canvasInteractionBenchmarkCounters = {})
    recordCanvasGraphCommand('move')
    recordCanvasGraphCommand('move')
    recordCanvasInteractionCount('activeResources', 2)
    recordCanvasInteractionCount('activeResources', -1)
    recordCanvasInteractionCount('graphCommands')
    recordCanvasInteractionCount('activeResources', NaN)
    expect(counters).toEqual({
      graphCommandCount: 2,
      graphCommands: { move: 2 },
      activeResources: 1,
    })
  })

  test('keeps exact cumulative totals and bounded recent timing percentiles', () => {
    window.__canvasInteractionBenchmarkCounters = {}
    const now = vi.spyOn(performance, 'now')
    for (const value of [...Array.from({ length: 300 }, (_, index) => index + 1), -1, Infinity]) {
      now.mockReturnValueOnce(0).mockReturnValueOnce(value)
      beginCanvasInteractionTiming('hydrate')?.()
    }
    expect(window.__canvasInteractionBenchmarkCounters.runtimeTimingMs?.hydrate).toEqual({
      count: 300,
      totalMs: 45150,
      maxMs: 300,
      sampleCount: 256,
      p50Ms: 172,
      p95Ms: 288,
    })
  })

  test('records latest gauges and rejects invalid resource snapshots', () => {
    window.__canvasInteractionBenchmarkCounters = {}
    recordCanvasInteractionGauge('previewPixels', 1200)
    recordCanvasInteractionGauge('previewPixels', 400)
    recordCanvasInteractionGauge('previewPixels', -1)
    recordCanvasInteractionGauge('previewPixels', Infinity)
    expect(window.__canvasInteractionBenchmarkCounters.gauges).toEqual({ previewPixels: 400 })
  })

  test('starts a fresh distribution when the collection is reset', () => {
    window.__canvasInteractionBenchmarkCounters = {}
    const now = vi.spyOn(performance, 'now')
    now.mockReturnValueOnce(0).mockReturnValueOnce(1000)
    beginCanvasInteractionTiming('decode')?.()
    window.__canvasInteractionBenchmarkCounters = {}
    now.mockReturnValueOnce(0).mockReturnValueOnce(1)
    beginCanvasInteractionTiming('decode')?.()
    expect(window.__canvasInteractionBenchmarkCounters.runtimeTimingMs?.decode).toMatchObject({
      count: 1,
      p95Ms: 1,
      maxMs: 1,
    })
  })

  test('drops stale completions and outcomes when a collection is replaced', () => {
    const original = (window.__canvasInteractionBenchmarkCounters = {})
    const finish = beginCanvasInteractionTiming('decode')
    const replacement = (window.__canvasInteractionBenchmarkCounters = {})
    const now = vi.spyOn(performance, 'now')

    finish?.({ count: 'decodeReady' })

    expect(original).toEqual({})
    expect(replacement).toEqual({})
    expect(now).not.toHaveBeenCalled()
  })

  test('records failed outcomes without adding successful decode timing', () => {
    window.__canvasInteractionBenchmarkCounters = {}
    beginCanvasInteractionTiming('decode')?.({ count: 'decodeFailed', shouldRecordTiming: false })
    expect(window.__canvasInteractionBenchmarkCounters).toEqual({ decodeFailed: 1 })
  })
})
