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
import { formatIdeaAmount } from '../../../../shared/utils/ideaCurrency'
import { i18n } from '../../../../shared/i18n'
import { readinessErrorCopy } from '../../../../shared/errors/errorCatalog'
import { fetchCanvasBillingReadiness } from './canvasBillingReadinessResource'
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

type CanvasBillingReadinessRequest = Parameters<typeof fetchCanvasBillingReadiness>[1]

export type CanvasBillingReadinessStore = {
  ensureFeature: (
    feature: WorkflowFeature,
    options?: { force?: boolean; request?: CanvasBillingReadinessRequest },
  ) => Promise<void>
  getFeatureState: (
    feature: WorkflowFeature,
    request?: CanvasBillingReadinessRequest,
  ) => CanvasBillingQuoteState
  applyReadiness: (readiness: SubmitTaskReadinessData) => void
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
    now?: () => number
    cacheMs?: number
  } = {},
): CanvasBillingReadinessStore {
  const fetchReadiness = options.fetchReadiness ?? fetchCanvasBillingReadiness
  const now = options.now ?? Date.now
  const cacheMs = options.cacheMs ?? 60_000
  const states = reactive(new Map<string, CanvasBillingQuoteState>())
  const pending = new Map<string, Promise<void>>()
  const loadedAt = new Map<string, number>()

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
    states.set(key, state)
    loadedAt.set(key, now())
  }

  function applyReadiness(readiness: SubmitTaskReadinessData): void {
    if (!isWorkflowFeature(readiness.feature)) return
    applyReadinessForKey(buildReadinessStateKey(readiness.feature), readiness)
  }

  async function ensureFeature(
    feature: WorkflowFeature,
    requestOptions: {
      force?: boolean
      request?: CanvasBillingReadinessRequest
    } = {},
  ): Promise<void> {
    const stateKey = buildReadinessStateKey(feature, requestOptions.request)
    const lastLoadedAt = loadedAt.get(stateKey) ?? 0
    if (!requestOptions.force && lastLoadedAt > 0 && now() - lastLoadedAt < cacheMs) return
    const existing = pending.get(stateKey)
    if (existing) return existing

    const previous = states.get(stateKey) ?? EMPTY_STATES[feature]
    states.set(stateKey, { ...previous, status: 'loading', reason: undefined })
    const request = fetchReadiness(feature, requestOptions.request)
      .then((readiness) => applyReadinessForKey(stateKey, readiness))
      .catch(() => {
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

  return {
    ensureFeature,
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
