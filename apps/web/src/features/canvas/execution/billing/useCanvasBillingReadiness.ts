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

export type CanvasBillingReadinessStore = {
  ensureFeature: (
    feature: WorkflowFeature,
    options?: { force?: boolean; request?: Parameters<typeof fetchCanvasBillingReadiness>[1] },
  ) => Promise<void>
  getFeatureState: (feature: WorkflowFeature) => CanvasBillingQuoteState
  applyReadiness: (readiness: SubmitTaskReadinessData) => void
}

const EMPTY_STATES = Object.fromEntries(
  WORKFLOW_FEATURE_SLUGS.map((feature) => [feature, { feature, status: 'idle' as const }]),
) as Record<WorkflowFeature, CanvasBillingQuoteState>

export const canvasBillingReadinessKey: InjectionKey<CanvasBillingReadinessStore> = Symbol(
  'canvas-billing-readiness',
)

function normalizeBlockedReason(message: string | undefined, fallback: string): string {
  const normalized = `${message ?? ''}`.trim()
  if (!normalized) return fallback
  if (/Idea.*不足|余额.*不足|insufficient/i.test(normalized)) return 'Idea不足'
  if (/未配置|不存在/.test(normalized) && /SKU|价格|计价|Idea/.test(normalized)) {
    return '价格未配置'
  }
  if (/停用|禁用|未启用/.test(normalized) && /价格|计价|SKU/.test(normalized)) {
    return '价格已停用'
  }
  return normalized
}

function isWorkflowFeature(value: string | undefined): value is WorkflowFeature {
  return WORKFLOW_FEATURE_SLUGS.includes(value as WorkflowFeature)
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
  const states = reactive({ ...EMPTY_STATES }) as Record<WorkflowFeature, CanvasBillingQuoteState>
  const pending = new Map<WorkflowFeature, Promise<void>>()
  const loadedAt = new Map<WorkflowFeature, number>()

  function applyReadiness(readiness: SubmitTaskReadinessData): void {
    if (!isWorkflowFeature(readiness.feature)) return
    const feature = readiness.feature
    const billing = readiness.billing
    if (billing && !billing.sufficient) {
      states[feature] = {
        feature,
        status: 'ready',
        unitCredits: billing.unit_credits,
        totalCredits: billing.total_credits,
        availableCredits: billing.available_credits,
        priceVersionId: billing.price_version_id,
        sufficient: false,
        reason: `Idea不足（需要 ${formatIdeaAmount(billing.total_credits)}，当前 ${formatIdeaAmount(billing.available_credits)}）`,
      }
      return
    }
    if (!readiness.ready) {
      states[feature] = {
        feature,
        status: 'blocked',
        reason: normalizeBlockedReason(readiness.message, '当前操作暂不可用'),
      }
      return
    }
    if (!billing) {
      states[feature] = {
        feature,
        status: 'error',
        reason: 'Idea计费信息暂不可用',
      }
      return
    }
    states[feature] = {
      feature,
      status: 'ready',
      unitCredits: billing.unit_credits,
      totalCredits: billing.total_credits,
      availableCredits: billing.available_credits,
      priceVersionId: billing.price_version_id,
      sufficient: billing.sufficient,
    }
    loadedAt.set(feature, now())
  }

  async function ensureFeature(
    feature: WorkflowFeature,
    requestOptions: {
      force?: boolean
      request?: Parameters<typeof fetchCanvasBillingReadiness>[1]
    } = {},
  ): Promise<void> {
    const lastLoadedAt = loadedAt.get(feature) ?? 0
    if (!requestOptions.force && lastLoadedAt > 0 && now() - lastLoadedAt < cacheMs) return
    const existing = pending.get(feature)
    if (existing) return existing

    const previous = states[feature]
    states[feature] = { ...previous, status: 'loading', reason: undefined }
    const request = fetchReadiness(feature, requestOptions.request)
      .then(applyReadiness)
      .catch((error: unknown) => {
        states[feature] = {
          feature,
          status: 'error',
          reason: normalizeBlockedReason(
            error instanceof Error ? error.message : undefined,
            'Idea计费信息暂不可用',
          ),
        }
      })
      .finally(() => {
        pending.delete(feature)
      })
    pending.set(feature, request)
    return request
  }

  return {
    ensureFeature,
    getFeatureState: (feature) => states[feature],
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
  return computed(() => (store ? store.getFeatureState(feature()) : null))
}
