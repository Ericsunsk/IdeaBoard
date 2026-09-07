import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_EVENT,
  RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_KEY,
  type RetouchPlanExecutorBrowserDiagnostic,
  type RetouchPlanExecutorDiagnosticsStore,
} from './imageRetouchRuntimeDiagnostics'
import type { ImageRetouchPlanExecutionEventReporter } from './planExecutorTypes'
import type { ImageProcessingBackend, ImageProcessingRuntime } from './types'

const { createImageRetouchProcessingBackendMock, createImageProcessingRuntimeMock } = vi.hoisted(
  () => ({
    createImageRetouchProcessingBackendMock: vi.fn(),
    createImageProcessingRuntimeMock: vi.fn(),
  }),
)

vi.mock('./ImageProcessingRuntime', () => ({
  createImageProcessingRuntime: createImageProcessingRuntimeMock,
}))

vi.mock('./imageProcessingBackends', () => ({
  createImageRetouchProcessingBackend: createImageRetouchProcessingBackendMock,
}))

type DiagnosticsWindow = Window & {
  [RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_KEY]?: RetouchPlanExecutorDiagnosticsStore
}

afterEach(async () => {
  const runtimeModule = await import('./imageRetouchProcessingRuntime')
  runtimeModule.disposeImageRetouchProcessingRuntime()
  delete (window as DiagnosticsWindow)[RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_KEY]
  vi.restoreAllMocks()
  createImageRetouchProcessingBackendMock.mockReset()
  createImageProcessingRuntimeMock.mockReset()
})

describe('image retouch processing runtime', () => {
  it('wires default plan executor diagnostics to browser-observable events', async () => {
    const runtimeModule = await import('./imageRetouchProcessingRuntime')
    const backend = createBackendStub()
    const runtime = createRuntimeStub()
    const eventDetails: RetouchPlanExecutorBrowserDiagnostic[] = []
    const listener = vi.fn<EventListener>((event) => {
      eventDetails.push((event as CustomEvent<RetouchPlanExecutorBrowserDiagnostic>).detail)
    })
    createImageRetouchProcessingBackendMock.mockReturnValue(backend)
    createImageProcessingRuntimeMock.mockReturnValue(runtime)
    window.addEventListener(RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_EVENT, listener)

    const resolvedRuntime = runtimeModule.getImageRetouchProcessingRuntime()

    expect(resolvedRuntime).toBe(runtime)
    expect(createImageRetouchProcessingBackendMock).toHaveBeenCalledWith({
      onPlanExecutorEvent: expect.any(Function),
    })
    expect(createImageProcessingRuntimeMock).toHaveBeenCalledWith({ backend })

    const reporter = createImageRetouchProcessingBackendMock.mock.calls[0]?.[0]
      ?.onPlanExecutorEvent as ImageRetouchPlanExecutionEventReporter | undefined
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
