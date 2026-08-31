import {
  computed,
  inject,
  onScopeDispose,
  provide,
  reactive,
  watchEffect,
  type ComputedRef,
  type InjectionKey,
} from 'vue'
import {
  WORKFLOW_FEATURE_SLUGS,
  type WorkflowFeature,
} from '../../../../shared/domain/workflowFeatures'
import type { SubmitTaskReadinessData } from '../../../../shared/api/task'
import { formatIdeaAmount } from '../../../../shared/i18n/userIdeaCurrency'
import { i18n } from '../../../../shared/i18n'
import { readinessErrorCopy } from '../../../../shared/errors/errorCatalog'
import {
  fetchCanvasBillingReadiness,
  fetchCanvasBillingReadinessMatrix,
} from './canvasBillingReadinessResource'
import { subscribeCanvasTaskReadiness } from './taskReadinessEvents'

export type CanvasBillingQuoteState = {
  feature: WorkflowFeature
  status: 'idle' | 'loading' | 'ready' | 'blocked' | 'error'
  unitCredits?: number
  totalCredits?: number
  availableCredits?: number
  priceVersionId?: number
  sufficient?: boolean
  reason?: string
}

type CanvasBillingReadinessFetcher = (
  feature: WorkflowFeature,
  request?: Parameters<typeof fetchCanvasBillingReadiness>[1],
) => Promise<SubmitTaskReadinessData>
type CanvasBillingReadinessMatrixFetcher = (
  requests: NonNullable<CanvasBillingReadinessRequest>[],
) => Promise<SubmitTaskReadinessData[]>

type CanvasBillingReadinessRequest = Parameters<typeof fetchCanvasBillingReadiness>[1]

export type CanvasBillingReadinessStore = {
  ensureFeature: (
    feature: WorkflowFeature,
    options?: { force?: boolean; request?: CanvasBillingReadinessRequest },
  ) => Promise<void>
  ensureMatrix?: (entries: Array<{ feature: WorkflowFeature; request: NonNullable<CanvasBillingReadinessRequest> }>) => Promise<void>
  getFeatureState: (
    feature: WorkflowFeature,
    request?: CanvasBillingReadinessRequest,
  ) => CanvasBillingQuoteState
  applyReadiness: (readiness: SubmitTaskReadinessData, request?: CanvasBillingReadinessRequest) => void
}

const EMPTY_STATES = Object.fromEntries(
  WORKFLOW_FEATURE_SLUGS.map((feature) => [feature, { feature, status: 'idle' as const }]),
) as Record<WorkflowFeature, CanvasBillingQuoteState>

export const canvasBillingReadinessKey: InjectionKey<CanvasBillingReadinessStore> = Symbol(
  'canvas-billing-readiness',
)

function isWorkflowFeature(value: string | undefined): value is WorkflowFeature {
  return WORKFLOW_FEATURE_SLUGS.includes(value as WorkflowFeature)
}

function buildReadinessStateKey(
  feature: WorkflowFeature,
  request?: CanvasBillingReadinessRequest,
): string {
  return request === undefined ? feature : `${feature}:${JSON.stringify(request)}`
}

export function createCanvasBillingReadinessStore(
  options: {
    fetchReadiness?: CanvasBillingReadinessFetcher
    fetchReadinessMatrix?: CanvasBillingReadinessMatrixFetcher
    now?: () => number
    cacheMs?: number
  } = {},
): CanvasBillingReadinessStore {
  const fetchReadiness = options.fetchReadiness ?? fetchCanvasBillingReadiness
  const customFetcher = options.fetchReadiness !== undefined
  const fetchReadinessMatrix = options.fetchReadinessMatrix ?? fetchCanvasBillingReadinessMatrix
  const now = options.now ?? Date.now
  const cacheMs = options.cacheMs ?? 60_000
  const states = reactive(new Map<string, CanvasBillingQuoteState>())
  const pending = new Map<string, Promise<void>>()
  const pendingMatrices = new Map<string, Promise<void>>()
  const loadedAt = new Map<string, number>()
  const failedAt = new Map<string, number>()

  function resolveReadinessState(
    readiness: SubmitTaskReadinessData,
  ): CanvasBillingQuoteState | null {
    if (!isWorkflowFeature(readiness.feature)) return null
    const feature = readiness.feature
    const billing = readiness.billing
    if (billing && !billing.sufficient) {
      return {
        feature,
        status: 'ready',
        unitCredits: billing.unit_credits,
        totalCredits: billing.total_credits,
        availableCredits: billing.available_credits,
        priceVersionId: billing.price_version_id,
        sufficient: false,
        reason: i18n.global.t('billing.insufficientBalanceDetail', {
          required: formatIdeaAmount(billing.total_credits),
          available: formatIdeaAmount(billing.available_credits),
        }),
      }
    }
    if (!readiness.ready) {
      return {
        feature,
        status: 'blocked',
        reason: readinessErrorCopy(readiness.error_code),
      }
    }
    if (!billing) {
      return {
        feature,
        status: 'error',
        reason: i18n.global.t('common.errors.billingInfoUnavailable'),
      }
    }
    return {
      feature,
      status: 'ready',
      unitCredits: billing.unit_credits,
      totalCredits: billing.total_credits,
      availableCredits: billing.available_credits,
      priceVersionId: billing.price_version_id,
      sufficient: billing.sufficient,
    }
  }

  function applyReadinessForKey(key: string, readiness: SubmitTaskReadinessData): void {
    const state = resolveReadinessState(readiness)
    if (!state) return
    failedAt.delete(key)
    states.set(key, state)
    loadedAt.set(key, now())
  }

  function applyReadiness(readiness: SubmitTaskReadinessData, request?: CanvasBillingReadinessRequest): void {
    if (!isWorkflowFeature(readiness.feature)) return
    applyReadinessForKey(buildReadinessStateKey(readiness.feature, request), readiness)
  }

  async function ensureFeature(
    feature: WorkflowFeature,
    requestOptions: {
      force?: boolean
      request?: CanvasBillingReadinessRequest
    } = {},
  ): Promise<void> {
    const stateKey = buildReadinessStateKey(feature, requestOptions.request)
    const lastFailureAt = failedAt.get(stateKey)
    if (lastFailureAt !== undefined && now() - lastFailureAt < cacheMs) return
    const lastLoadedAt = loadedAt.get(stateKey) ?? 0
    if (!requestOptions.force && lastLoadedAt > 0 && now() - lastLoadedAt < cacheMs) return
    const existing = pending.get(stateKey)
    if (existing) return existing

    const previous = states.get(stateKey) ?? EMPTY_STATES[feature]
    states.set(stateKey, { ...previous, status: 'loading', reason: undefined })
    const request = fetchReadiness(feature, requestOptions.request)
      .then((readiness) => applyReadinessForKey(stateKey, readiness))
      .catch(() => {
        // Optional Billing/provider integrations can be intentionally unavailable.
        // Cache the failure briefly so reactive quote consumers do not create a request storm.
        failedAt.set(stateKey, now())
        states.set(stateKey, {
          feature,
          status: 'error',
          // Transport failures carry no user-safe detail; show the generic copy.
          reason: i18n.global.t('common.errors.billingInfoUnavailable'),
        })
      })
      .finally(() => {
        pending.delete(stateKey)
      })
    pending.set(stateKey, request)
    return request
  }

  async function ensureMatrix(
    entries: Array<{ feature: WorkflowFeature; request: NonNullable<CanvasBillingReadinessRequest> }>,
  ): Promise<void> {
    // Custom fetchers are used by isolated consumers/tests that only implement the
    // single-feature contract; preserve that adapter seam without issuing a real request.
    if (customFetcher && options.fetchReadinessMatrix === undefined) {
      await Promise.all(entries.map(({ feature, request }) => ensureFeature(feature, { request })))
      return
    }
    const stale = entries.filter(({ feature, request }) => {
      const key = buildReadinessStateKey(feature, request)
      const loaded = loadedAt.get(key) ?? 0
      const failed = failedAt.get(key) ?? 0
      return now() - loaded >= cacheMs && now() - failed >= cacheMs
    })
    if (stale.length === 0) return
    const batchKey = stale
      .map(({ feature, request }) => buildReadinessStateKey(feature, request))
      .sort()
      .join('|')
    const pendingBatch = pendingMatrices.get(batchKey)
    if (pendingBatch) return pendingBatch
    const requests = stale.map((entry) => entry.request)
    const batch = fetchReadinessMatrix(requests)
      .then((values) => {
        values.forEach((readiness, index) => {
          const entry = stale[index]
          if (entry) applyReadinessForKey(buildReadinessStateKey(entry.feature, entry.request), readiness)
        })
      })
      .catch(() => {
        const timestamp = now()
        stale.forEach(({ feature, request }) => {
          const key = buildReadinessStateKey(feature, request)
          failedAt.set(key, timestamp)
          states.set(key, { feature, status: 'error', reason: i18n.global.t('common.errors.billingInfoUnavailable') })
        })
      })
      .finally(() => pendingMatrices.delete(batchKey))
    pendingMatrices.set(batchKey, batch)
    return batch
  }

  return {
    ensureFeature,
    ensureMatrix,
    getFeatureState: (feature, request) =>
      states.get(buildReadinessStateKey(feature, request)) ?? EMPTY_STATES[feature],
    applyReadiness,
  }
}

export function provideCanvasBillingReadiness(
  store: CanvasBillingReadinessStore = createCanvasBillingReadinessStore(),
): CanvasBillingReadinessStore {
  provide(canvasBillingReadinessKey, store)
  const unsubscribe = subscribeCanvasTaskReadiness(store.applyReadiness)
  onScopeDispose(unsubscribe)
  return store
}

export function useCanvasBillingReadinessStore(): CanvasBillingReadinessStore | null {
  return inject(canvasBillingReadinessKey, null)
}

export function useCanvasBillingQuote(
  feature: () => WorkflowFeature,
  enabled: () => boolean = () => true,
  request: () => Parameters<typeof fetchCanvasBillingReadiness>[1] = () => undefined,
): ComputedRef<CanvasBillingQuoteState | null> {
  const store = useCanvasBillingReadinessStore()
  watchEffect(() => {
    if (!store || !enabled()) return
    const readinessRequest = request()
    void store.ensureFeature(feature(), {
      request: readinessRequest,
      // Readiness quotes include quantity and resolution. Dynamic node controls
      // must bypass the feature-level cache when they provide a concrete request.
      force: readinessRequest !== undefined,
    })
  })
  return computed(() => {
    if (!store) return null
    return store.getFeatureState(feature(), request())
  })
}
