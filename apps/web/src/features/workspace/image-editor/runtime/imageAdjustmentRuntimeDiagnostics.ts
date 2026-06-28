import type {
  ImageAdjustmentPlanExecutionEvent,
  ImageAdjustmentPlanExecutionEventReporter,
} from './imageAdjustmentPlanExecutor'

export const IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_EVENT =
  'ideaboard:image-editor:plan-executor-diagnostic'

export const IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_KEY =
  '__ideaBoardImageEditorPlanExecutorDiagnostics'

export const DEFAULT_IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_LIMIT = 100

export type ImageEditorPlanExecutorBrowserDiagnostic = ImageAdjustmentPlanExecutionEvent & {
  sequence: number
  timestampMs: number
}

export type ImageEditorPlanExecutorDiagnosticsStore = {
  events: ImageEditorPlanExecutorBrowserDiagnostic[]
  clear(): void
}

type DiagnosticsWindow = Window & {
  [IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_KEY]?: ImageEditorPlanExecutorDiagnosticsStore
}

export type CreateBrowserPlanExecutorDiagnosticsReporterOptions = {
  maxEvents?: number
  now?: () => number
}

export function createBrowserPlanExecutorDiagnosticsReporter(
  options: CreateBrowserPlanExecutorDiagnosticsReporterOptions = {},
): ImageAdjustmentPlanExecutionEventReporter {
  const maxEvents = normalizeMaxEvents(options.maxEvents)

  return (event) => {
    const browserWindow = getDiagnosticsWindow()
    if (!browserWindow) return

    const store = getOrCreateBrowserPlanExecutorDiagnosticsStore(browserWindow)
    const diagnostic: ImageEditorPlanExecutorBrowserDiagnostic = {
      ...event,
      sequence: resolveNextSequence(store),
      timestampMs: resolveTimestampMs(browserWindow, options.now),
    }

    store.events.push(diagnostic)
    trimDiagnostics(store, maxEvents)
    dispatchBrowserDiagnostic(browserWindow, diagnostic)
  }
}

export function getBrowserPlanExecutorDiagnosticsStore():
  | ImageEditorPlanExecutorDiagnosticsStore
  | null {
  const browserWindow = getDiagnosticsWindow()
  if (!browserWindow) return null
  return getOrCreateBrowserPlanExecutorDiagnosticsStore(browserWindow)
}

function getDiagnosticsWindow(): DiagnosticsWindow | null {
  if (typeof window === 'undefined') return null
  return window as DiagnosticsWindow
}

function getOrCreateBrowserPlanExecutorDiagnosticsStore(
  browserWindow: DiagnosticsWindow,
): ImageEditorPlanExecutorDiagnosticsStore {
  const current = browserWindow[IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_KEY]
  if (isDiagnosticsStore(current)) return current

  const store: ImageEditorPlanExecutorDiagnosticsStore = {
    events: [],
    clear() {
      store.events.length = 0
    },
  }

  Object.defineProperty(browserWindow, IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_KEY, {
    configurable: true,
    enumerable: false,
    writable: true,
    value: store,
  })
  return store
}

function isDiagnosticsStore(value: unknown): value is ImageEditorPlanExecutorDiagnosticsStore {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Partial<ImageEditorPlanExecutorDiagnosticsStore>
  return Array.isArray(candidate.events) && typeof candidate.clear === 'function'
}

function resolveNextSequence(store: ImageEditorPlanExecutorDiagnosticsStore) {
  return (store.events.at(-1)?.sequence ?? 0) + 1
}

function resolveTimestampMs(browserWindow: DiagnosticsWindow, now: (() => number) | undefined) {
  const timestamp = now?.() ?? browserWindow.performance?.now?.() ?? Date.now()
  return Number.isFinite(timestamp) ? timestamp : Date.now()
}

function trimDiagnostics(store: ImageEditorPlanExecutorDiagnosticsStore, maxEvents: number) {
  if (store.events.length <= maxEvents) return
  store.events.splice(0, store.events.length - maxEvents)
}

function normalizeMaxEvents(maxEvents: number | undefined) {
  if (maxEvents === undefined) return DEFAULT_IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_LIMIT
  if (!Number.isFinite(maxEvents) || maxEvents < 1) {
    return DEFAULT_IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_LIMIT
  }
  return Math.floor(maxEvents)
}

function dispatchBrowserDiagnostic(
  browserWindow: DiagnosticsWindow,
  diagnostic: ImageEditorPlanExecutorBrowserDiagnostic,
) {
  if (typeof CustomEvent !== 'function') return
  browserWindow.dispatchEvent(
    new CustomEvent(IMAGE_EDITOR_PLAN_EXECUTOR_DIAGNOSTICS_EVENT, {
      detail: diagnostic,
    }),
  )
}
