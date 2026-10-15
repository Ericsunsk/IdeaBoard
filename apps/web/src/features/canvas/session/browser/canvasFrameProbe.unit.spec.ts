import { afterEach, describe, expect, it, vi } from 'vitest'

function installAnimationFrameMock() {
  const callbacks = new Map<number, FrameRequestCallback>()
  let frameId = 0

  const requestAnimationFrameMock = vi.fn((callback: FrameRequestCallback) => {
    frameId += 1
    callbacks.set(frameId, callback)
    return frameId
  })
  const cancelAnimationFrameMock = vi.fn((id: number) => {
    callbacks.delete(id)
  })

  vi.stubGlobal('requestAnimationFrame', requestAnimationFrameMock)
  vi.stubGlobal('cancelAnimationFrame', cancelAnimationFrameMock)

  return {
    requestAnimationFrameMock,
    cancelAnimationFrameMock,
    flush(timestamp: number) {
      const pendingCallbacks = Array.from(callbacks.entries())
      callbacks.clear()
      for (const [, callback] of pendingCallbacks) {
        callback(timestamp)
      }
    },
  }
}

describe('canvasFrameProbe', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllEnvs()
    vi.unstubAllGlobals()
    vi.resetModules()
  })

  it('stays inert when the canvas perf probe flag is disabled', async () => {
    vi.stubEnv('VITE_CANVAS_PERF_PROBE', 'false')
    vi.doMock('./canvasPerfProbe', () => ({ isCanvasPerfProbeEnabled: () => false }))
    const frame = installAnimationFrameMock()
    const metadata = vi.fn(() => ({ phase: 'disabled' }))
    const info = vi.spyOn(console, 'info').mockImplementation(() => {})
    const { beginCanvasFrameProbe } = await import('./canvasFrameProbe')

    const session = beginCanvasFrameProbe('pan', metadata)
    session.mark('pointermove', metadata)

    expect(session.end(metadata)).toBeUndefined()
    expect(frame.requestAnimationFrameMock).not.toHaveBeenCalled()
    expect(frame.cancelAnimationFrameMock).not.toHaveBeenCalled()
    expect(metadata).not.toHaveBeenCalled()
    expect(info).not.toHaveBeenCalled()
  })

  it('samples frame deltas and logs a frame summary when enabled', async () => {
    vi.stubEnv('VITE_CANVAS_PERF_PROBE', 'true')
    vi.doMock('./canvasPerfProbe', () => ({ isCanvasPerfProbeEnabled: () => true }))
    vi.spyOn(performance, 'now')
      .mockReturnValueOnce(100)
      .mockReturnValueOnce(120)
      .mockReturnValueOnce(180)
    const frame = installAnimationFrameMock()
    const info = vi.spyOn(console, 'info').mockImplementation(() => {})
    const { beginCanvasFrameProbe } = await import('./canvasFrameProbe')

    const session = beginCanvasFrameProbe('pan', { nodeCount: 3 })
    frame.flush(100)
    frame.flush(116)
    session.mark('pointermove', { selectedCount: 2 })
    frame.flush(150)

    const summary = session.end({ isCommitted: true })

    expect(summary).toEqual({
      scene: 'pan',
      durationMs: 80,
      avgFrameMs: 25,
      p95FrameMs: 34,
      maxFrameMs: 34,
      droppedFrames: 1,
      sampleCount: 2,
      retainedSampleCount: 2,
      droppedMarkCount: 0,
      p50FrameMs: 16,
      frameBudgetMs: 16.7,
      metadata: { nodeCount: 3, isCommitted: true },
      marks: [{ name: 'pointermove', atMs: 20, metadata: { selectedCount: 2 } }],
    })
    expect(info).toHaveBeenCalledTimes(1)
    expect(info.mock.calls[0]).toEqual([`[canvas:perf] frame pan 80ms`, summary])

    const requestCountAfterEnd = frame.requestAnimationFrameMock.mock.calls.length
    frame.flush(166)
    expect(frame.requestAnimationFrameMock).toHaveBeenCalledTimes(requestCountAfterEnd)
    expect(frame.cancelAnimationFrameMock).toHaveBeenCalled()
  })

  it('stops without logging when cleanup is called', async () => {
    vi.stubEnv('VITE_CANVAS_PERF_PROBE', 'true')
    vi.doMock('./canvasPerfProbe', () => ({ isCanvasPerfProbeEnabled: () => true }))
    vi.spyOn(performance, 'now').mockReturnValue(0)
    const frame = installAnimationFrameMock()
    const info = vi.spyOn(console, 'info').mockImplementation(() => {})
    const { beginCanvasFrameProbe } = await import('./canvasFrameProbe')

    const session = beginCanvasFrameProbe('zoom')
    frame.flush(0)
    session.cleanup()

    const requestCountAfterCleanup = frame.requestAnimationFrameMock.mock.calls.length
    frame.flush(16)

    expect(frame.cancelAnimationFrameMock).toHaveBeenCalled()
    expect(frame.requestAnimationFrameMock).toHaveBeenCalledTimes(requestCountAfterCleanup)
    expect(session.end()).toBeUndefined()
    expect(info).not.toHaveBeenCalled()
  })

  it('bounds retained samples and marks during long user-driven traces', async () => {
    vi.doMock('./canvasPerfProbe', () => ({ isCanvasPerfProbeEnabled: () => true }))
    const frame = installAnimationFrameMock()
    vi.spyOn(console, 'info').mockImplementation(() => {})
    const { beginCanvasFrameProbe } = await import('./canvasFrameProbe')
    const session = beginCanvasFrameProbe('pan')
    frame.flush(0)
    frame.flush(1000)
    for (let index = 1; index <= 5000; index += 1) frame.flush(1000 + index * 16)
    const metadata = vi.fn(() => ({ phase: 'move' }))
    for (let index = 0; index < 300; index += 1) session.mark('move', metadata)
    const summary = session.end()
    expect(summary).toMatchObject({
      sampleCount: 5001,
      retainedSampleCount: 4096,
      p50FrameMs: 16,
      p95FrameMs: 16,
      maxFrameMs: 1000,
      droppedMarkCount: 44,
    })
    expect(summary?.marks).toHaveLength(256)
    expect(metadata).toHaveBeenCalledTimes(256)
  })

  it('stays inert when requestAnimationFrame is unavailable', async () => {
    vi.stubEnv('VITE_CANVAS_PERF_PROBE', 'true')
    vi.doMock('./canvasPerfProbe', () => ({ isCanvasPerfProbeEnabled: () => true }))
    vi.stubGlobal('requestAnimationFrame', undefined)
    const cancelAnimationFrameMock = vi.fn()
    vi.stubGlobal('cancelAnimationFrame', cancelAnimationFrameMock)
    const info = vi.spyOn(console, 'info').mockImplementation(() => {})
    const { beginCanvasFrameProbe } = await import('./canvasFrameProbe')

    const session = beginCanvasFrameProbe('load-hydrate')

    expect(session.end()).toBeUndefined()
    expect(cancelAnimationFrameMock).not.toHaveBeenCalled()
    expect(info).not.toHaveBeenCalled()
  })
})
