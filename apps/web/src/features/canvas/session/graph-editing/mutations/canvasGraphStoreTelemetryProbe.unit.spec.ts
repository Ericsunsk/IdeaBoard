import { afterEach, describe, expect, it } from 'vitest'
import { createEmptyBoardSnapshot } from '../../../../board'
import { CanvasGraphStore } from '../../../core/graph/canvasGraphStore'
import {
  noopCanvasGraphStoreTelemetry,
  type CanvasGraphStoreTelemetry,
} from '../../../core/graph/canvasGraphStoreTelemetry'
import { canvasGraphStoreTelemetryProbe } from './canvasGraphStoreTelemetryProbe'

describe('canvasGraphStoreTelemetryProbe', () => {
  afterEach(() => {
    delete window.__canvasInteractionBenchmarkCounters
  })

  it('measures both constructor and explicit hydrate paths during a benchmark', () => {
    window.__canvasInteractionBenchmarkCounters = {}
    const store = CanvasGraphStore.fromSnapshot(
      createEmptyBoardSnapshot(),
      canvasGraphStoreTelemetryProbe,
    )

    store.hydrate(createEmptyBoardSnapshot())

    expect(window.__canvasInteractionBenchmarkCounters.runtimeTimingMs).toMatchObject({
      'graph-store.hydrate': { count: 2 },
    })
  })

  it('records commit counters matching the legacy inline behavior', () => {
    window.__canvasInteractionBenchmarkCounters = {}
    const telemetry: CanvasGraphStoreTelemetry = canvasGraphStoreTelemetryProbe
    const store = CanvasGraphStore.fromSnapshot(createEmptyBoardSnapshot(), telemetry)

    store
      .beginTransaction('node-update', { recordHistory: true, markDirty: true })
      .addNode({
        id: 1,
        type: 'prompt',
        x: 40,
        y: 40,
        width: 280,
        text: '',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      })
      .commit()

    expect(window.__canvasInteractionBenchmarkCounters.graphCommitCount).toBe(1)
    expect(window.__canvasInteractionBenchmarkCounters.graphHistoryCommitCount).toBe(1)
    expect(window.__canvasInteractionBenchmarkCounters.graphDirtyCommitCount).toBe(1)
  })

  it('keeps the noop telemetry side-effect free', () => {
    window.__canvasInteractionBenchmarkCounters = {}
    const store = CanvasGraphStore.fromSnapshot(
      createEmptyBoardSnapshot(),
      noopCanvasGraphStoreTelemetry,
    )

    store.hydrate(createEmptyBoardSnapshot())

    expect(window.__canvasInteractionBenchmarkCounters.runtimeTimingMs).toBeUndefined()
    expect(window.__canvasInteractionBenchmarkCounters.graphCommitCount).toBeUndefined()
  })
})
