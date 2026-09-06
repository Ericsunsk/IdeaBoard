import type {
  ImageAdjustmentPlanExecutionEvent,
  ImageAdjustmentPlanExecutionEventReporter,
} from './planExecutorTypes'

export const RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_EVENT = 'ideaboard:retouch:plan-executor-diagnostic'

export const RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_KEY = '__ideaBoardRetouchPlanExecutorDiagnostics'

export const DEFAULT_RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_LIMIT = 100

export type RetouchPlanExecutorBrowserDiagnostic = ImageAdjustmentPlanExecutionEvent & {
  sequence: number
  timestampMs: number
}

export type RetouchPlanExecutorDiagnosticsStore = {
  events: RetouchPlanExecutorBrowserDiagnostic[]
  clear(): void
}

type DiagnosticsWindow = Window & {
  [RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_KEY]?: RetouchPlanExecutorDiagnosticsStore
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
    const diagnostic: RetouchPlanExecutorBrowserDiagnostic = {
      ...event,
      sequence: resolveNextSequence(store),
      timestampMs: resolveTimestampMs(browserWindow, options.now),
    }

    store.events.push(diagnostic)
    trimDiagnostics(store, maxEvents)
    dispatchBrowserDiagnostic(browserWindow, diagnostic)
  }
}

export function getBrowserPlanExecutorDiagnosticsStore(): RetouchPlanExecutorDiagnosticsStore | null {
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
): RetouchPlanExecutorDiagnosticsStore {
  const current = browserWindow[RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_KEY]
  if (isDiagnosticsStore(current)) return current

  const store: RetouchPlanExecutorDiagnosticsStore = {
    events: [],
    clear() {
      store.events.length = 0
    },
  }

  Object.defineProperty(browserWindow, RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_KEY, {
    configurable: true,
    enumerable: false,
    writable: true,
    value: store,
  })
  return store
}

function isDiagnosticsStore(value: unknown): value is RetouchPlanExecutorDiagnosticsStore {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Partial<RetouchPlanExecutorDiagnosticsStore>
  return Array.isArray(candidate.events) && typeof candidate.clear === 'function'
}

function resolveNextSequence(store: RetouchPlanExecutorDiagnosticsStore) {
  return (store.events.at(-1)?.sequence ?? 0) + 1
}

function resolveTimestampMs(browserWindow: DiagnosticsWindow, now: (() => number) | undefined) {
  const timestamp = now?.() ?? browserWindow.performance?.now?.() ?? Date.now()
  return Number.isFinite(timestamp) ? timestamp : Date.now()
}

function trimDiagnostics(store: RetouchPlanExecutorDiagnosticsStore, maxEvents: number) {
  if (store.events.length <= maxEvents) return
  store.events.splice(0, store.events.length - maxEvents)
}

function normalizeMaxEvents(maxEvents: number | undefined) {
  if (maxEvents === undefined) return DEFAULT_RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_LIMIT
  if (!Number.isFinite(maxEvents) || maxEvents < 1) {
    return DEFAULT_RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_LIMIT
  }
  return Math.floor(maxEvents)
}

function dispatchBrowserDiagnostic(
  browserWindow: DiagnosticsWindow,
  diagnostic: RetouchPlanExecutorBrowserDiagnostic,
) {
  if (typeof CustomEvent !== 'function') return
  browserWindow.dispatchEvent(
    new CustomEvent(RETOUCH_PLAN_EXECUTOR_DIAGNOSTICS_EVENT, {
      detail: diagnostic,
    }),
  )
}
