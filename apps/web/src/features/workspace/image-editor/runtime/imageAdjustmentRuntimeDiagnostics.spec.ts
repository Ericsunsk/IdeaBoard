import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  createBrowserPlanExecutorDiagnosticsReporter,
  getBrowserPlanExecutorDiagnosticsStore,
  IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_EVENT,
  IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_KEY,
  type ImageEditorPlanExecutorBrowserDiagnostic,
  type ImageEditorPlanExecutorDiagnosticsStore,
} from './imageAdjustmentRuntimeDiagnostics'

type WorkerStartedPlanExecutorEvent = Omit<
  Extract<ImageEditorPlanExecutorBrowserDiagnostic, { kind: 'worker-plan-started' }>,
  'sequence' | 'timestampMs'
>

type DiagnosticsWindow = Window & {
  [IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_KEY]?: ImageEditorPlanExecutorDiagnosticsStore
}

afterEach(() => {
  delete (window as DiagnosticsWindow)[IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_KEY]
  vi.restoreAllMocks()
})

describe('image adjustment runtime diagnostics', () => {
  it('stores plan executor diagnostics on window and dispatches browser events', () => {
    const reporter = createBrowserPlanExecutorDiagnosticsReporter({ now: () => 42 })
    const eventDetails: ImageEditorPlanExecutorBrowserDiagnostic[] = []
    const listener = vi.fn<EventListener>((event) => {
      eventDetails.push((event as CustomEvent<ImageEditorPlanExecutorBrowserDiagnostic>).detail)
    })
    window.addEventListener(IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_EVENT, listener)

    reporter(createWorkerStartedEvent())

    const store = getBrowserPlanExecutorDiagnosticsStore()
    expect(store?.events).toEqual([
      {
        kind: 'worker-plan-started',
        planLength: 2,
        width: 320,
        height: 240,
        sequence: 1,
        timestampMs: 42,
      },
    ])
    expect(listener).toHaveBeenCalledOnce()
    expect(eventDetails).toEqual([store?.events[0]])

    window.removeEventListener(IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_EVENT, listener)
  })

  it('keeps diagnostics bounded and lets smoke tests clear the browser buffer', () => {
    const reporter = createBrowserPlanExecutorDiagnosticsReporter({
      maxEvents: 2,
      now: () => 7,
    })

    reporter(createWorkerStartedEvent({ planLength: 1 }))
    reporter(createWorkerStartedEvent({ planLength: 2 }))
    reporter(createWorkerStartedEvent({ planLength: 3 }))

    const store = getBrowserPlanExecutorDiagnosticsStore()
    expect(store?.events).toEqual([
      expect.objectContaining({ planLength: 2, sequence: 2 }),
      expect.objectContaining({ planLength: 3, sequence: 3 }),
    ])

    store?.clear()

    expect(store?.events).toEqual([])
  })
})

function createWorkerStartedEvent(
  overrides: Partial<WorkerStartedPlanExecutorEvent> = {},
): WorkerStartedPlanExecutorEvent {
  return {
    kind: 'worker-plan-started',
    planLength: 2,
    width: 320,
    height: 240,
    ...overrides,
  }
}
