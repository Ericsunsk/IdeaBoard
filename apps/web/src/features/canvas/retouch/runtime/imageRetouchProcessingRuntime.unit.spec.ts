import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_EVENT,
  RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_KEY,
  type RetouchPlanExecutorBrowserDiagnostic,
  type RetouchPlanExecutorDiagnosticsStore,
} from './imageRetouchRuntimeDiagnostics'
import type { ImageRetouchPlanExecutionEventReporter } from './planExecutorTypes'
import type { ImageProcessingBackend, ImageProcessingRuntime } from './types'

const { createHybridExecutorMock, createMainThreadBackendMock, createImageProcessingRuntimeMock } =
  vi.hoisted(() => ({
    createHybridExecutorMock: vi.fn(),
    createMainThreadBackendMock: vi.fn(),
    createImageProcessingRuntimeMock: vi.fn(),
  }))

vi.mock('./ImageProcessingRuntime', () => ({
  createImageProcessingRuntime: createImageProcessingRuntimeMock,
}))

vi.mock('./hybridPlanExecutor', () => ({
  createHybridImageRetouchPlanExecutor: createHybridExecutorMock,
}))

vi.mock('./mainThreadImageProcessingBackend', () => ({
  createMainThreadImageProcessingBackend: createMainThreadBackendMock,
}))

type DiagnosticsWindow = Window & {
  [RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_KEY]?: RetouchPlanExecutorDiagnosticsStore
}

afterEach(async () => {
  const runtimeModule = await import('./imageRetouchProcessingRuntime')
  runtimeModule.disposeImageRetouchProcessingRuntime()
  delete (window as DiagnosticsWindow)[RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_KEY]
  vi.restoreAllMocks()
  createHybridExecutorMock.mockReset()
  createMainThreadBackendMock.mockReset()
  createImageProcessingRuntimeMock.mockReset()
})

describe('image retouch processing runtime', () => {
  it('wires the main-thread backend to the hybrid plan executor', async () => {
    const runtimeModule = await import('./imageRetouchProcessingRuntime')
    const executor = { execute: vi.fn() }
    const backend = createBackendStub()
    const runtime = createRuntimeStub()
    createHybridExecutorMock.mockReturnValue(executor)
    createMainThreadBackendMock.mockReturnValue(backend)
    createImageProcessingRuntimeMock.mockReturnValue(runtime)

    expect(runtimeModule.getImageRetouchProcessingRuntime()).toBe(runtime)
    expect(createHybridExecutorMock).toHaveBeenCalledWith({
      onPlanExecutorEvent: expect.any(Function),
    })
    expect(createMainThreadBackendMock).toHaveBeenCalledWith({ planExecutor: executor })
    expect(createImageProcessingRuntimeMock).toHaveBeenCalledWith({ backend })
  })

  it('returns the same runtime until disposed', async () => {
    const runtimeModule = await import('./imageRetouchProcessingRuntime')
    const runtime = createRuntimeStub()
    createImageProcessingRuntimeMock.mockReturnValue(runtime)

    const first = runtimeModule.getImageRetouchProcessingRuntime()
    const second = runtimeModule.getImageRetouchProcessingRuntime()
    expect(first).toBe(runtime)
    expect(second).toBe(runtime)
    expect(createImageProcessingRuntimeMock).toHaveBeenCalledTimes(1)

    runtimeModule.disposeImageRetouchProcessingRuntime()
    runtimeModule.getImageRetouchProcessingRuntime()
    expect(createImageProcessingRuntimeMock).toHaveBeenCalledTimes(2)
  })

  it('wires default plan executor diagnostics to browser-observable events', async () => {
    const runtimeModule = await import('./imageRetouchProcessingRuntime')
    const backend = createBackendStub()
    const runtime = createRuntimeStub()
    const eventDetails: RetouchPlanExecutorBrowserDiagnostic[] = []
    const listener = vi.fn<EventListener>((event) => {
      eventDetails.push((event as CustomEvent<RetouchPlanExecutorBrowserDiagnostic>).detail)
    })
    createMainThreadBackendMock.mockReturnValue(backend)
    createImageProcessingRuntimeMock.mockReturnValue(runtime)
    window.addEventListener(RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_EVENT, listener)

    const resolvedRuntime = runtimeModule.getImageRetouchProcessingRuntime()

    expect(resolvedRuntime).toBe(runtime)
    expect(createHybridExecutorMock).toHaveBeenCalledWith({
      onPlanExecutorEvent: expect.any(Function),
    })

    const reporter = createHybridExecutorMock.mock.calls[0]?.[0]?.onPlanExecutorEvent as
      ImageRetouchPlanExecutionEventReporter | undefined
    reporter?.({
      kind: 'cpu-plan-completed',
      planLength: 4,
      width: 640,
      height: 480,
    })

    const store = (window as DiagnosticsWindow)[RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_KEY]
    expect(store?.events).toEqual([
      expect.objectContaining({
        kind: 'cpu-plan-completed',
        planLength: 4,
        width: 640,
        height: 480,
        sequence: 1,
      }),
    ])
    expect(eventDetails).toEqual([
      expect.objectContaining({
        kind: 'cpu-plan-completed',
        planLength: 4,
      }),
    ])

    window.removeEventListener(RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_EVENT, listener)
  })
})

function createBackendStub(): ImageProcessingBackend {
  return {
    renderFinal: vi.fn<ImageProcessingBackend['renderFinal']>(),
    renderPreview: vi.fn<ImageProcessingBackend['renderPreview']>(),
  }
}

function createRuntimeStub(): ImageProcessingRuntime {
  return {
    renderFinal: vi.fn<ImageProcessingRuntime['renderFinal']>(),
    renderPreview: vi.fn<ImageProcessingRuntime['renderPreview']>(),
    cancel: vi.fn<ImageProcessingRuntime['cancel']>(),
    dispose: vi.fn<ImageProcessingRuntime['dispose']>(),
  }
}
