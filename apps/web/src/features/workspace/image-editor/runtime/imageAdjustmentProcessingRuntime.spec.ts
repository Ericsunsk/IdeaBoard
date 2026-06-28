import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_EVENT,
  IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_KEY,
  type ImageEditorPlanExecutorBrowserDiagnostic,
  type ImageEditorPlanExecutorDiagnosticsStore,
} from './imageAdjustmentRuntimeDiagnostics'
import type { ImageAdjustmentPlanExecutionEventReporter } from './imageAdjustmentPlanExecutor'
import type { ImageProcessingBackend, ImageProcessingRuntime } from './types'

const { createImageAdjustmentProcessingBackendMock, createImageProcessingRuntimeMock } =
  vi.hoisted(() => ({
    createImageAdjustmentProcessingBackendMock: vi.fn(),
    createImageProcessingRuntimeMock: vi.fn(),
  }))

vi.mock('./ImageProcessingRuntime', () => ({
  createImageProcessingRuntime: createImageProcessingRuntimeMock,
}))

vi.mock('./imageProcessingBackends', () => ({
  createImageAdjustmentProcessingBackend: createImageAdjustmentProcessingBackendMock,
}))

type DiagnosticsWindow = Window & {
  [IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_KEY]?: ImageEditorPlanExecutorDiagnosticsStore
}

afterEach(async () => {
  const runtimeModule = await import('./imageAdjustmentProcessingRuntime')
  runtimeModule.disposeImageAdjustmentProcessingRuntime()
  delete (window as DiagnosticsWindow)[IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_KEY]
  vi.restoreAllMocks()
  createImageAdjustmentProcessingBackendMock.mockReset()
  createImageProcessingRuntimeMock.mockReset()
})

describe('image adjustment processing runtime', () => {
  it('wires default plan executor diagnostics to browser-observable events', async () => {
    const runtimeModule = await import('./imageAdjustmentProcessingRuntime')
    const backend = createBackendStub()
    const runtime = createRuntimeStub()
    const eventDetails: ImageEditorPlanExecutorBrowserDiagnostic[] = []
    const listener = vi.fn<EventListener>((event) => {
      eventDetails.push((event as CustomEvent<ImageEditorPlanExecutorBrowserDiagnostic>).detail)
    })
    createImageAdjustmentProcessingBackendMock.mockReturnValue(backend)
    createImageProcessingRuntimeMock.mockReturnValue(runtime)
    window.addEventListener(IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_EVENT, listener)

    const resolvedRuntime = runtimeModule.getImageAdjustmentProcessingRuntime()

    expect(resolvedRuntime).toBe(runtime)
    expect(createImageAdjustmentProcessingBackendMock).toHaveBeenCalledWith({
      onPlanExecutorEvent: expect.any(Function),
    })
    expect(createImageProcessingRuntimeMock).toHaveBeenCalledWith({ backend })

    const reporter = createImageAdjustmentProcessingBackendMock.mock.calls[0]?.[0]
      ?.onPlanExecutorEvent as ImageAdjustmentPlanExecutionEventReporter | undefined
    reporter?.({
      kind: 'cpu-plan-completed',
      planLength: 4,
      width: 640,
      height: 480,
    })

    const store = (window as DiagnosticsWindow)[IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_KEY]
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

    window.removeEventListener(IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_EVENT, listener)
  })
})

function createBackendStub(): ImageProcessingBackend {
  return {
    renderPreview: vi.fn<ImageProcessingBackend['renderPreview']>(),
    renderFinal: vi.fn<ImageProcessingBackend['renderFinal']>(),
  }
}

function createRuntimeStub(): ImageProcessingRuntime {
  return {
    renderPreview: vi.fn<ImageProcessingRuntime['renderPreview']>(),
    renderFinal: vi.fn<ImageProcessingRuntime['renderFinal']>(),
    cancel: vi.fn<ImageProcessingRuntime['cancel']>(),
    dispose: vi.fn<ImageProcessingRuntime['dispose']>(),
  }
}
