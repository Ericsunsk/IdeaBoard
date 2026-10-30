import {
  computed,
  inject,
  provide,
  reactive,
  watchEffect,
  type ComputedRef,
  type InjectionKey,
} from 'vue'
import { SERVICE_SLUGS, type Service } from '../../../../../core/contracts/imageServices'
import type { SubmitTaskReadinessData } from '../../../core/execution/canvasTaskRequest'
import { formatIdeaAmount } from '../../../../../core/i18n/userIdeaCurrency'
import { i18n } from '../../../../../core/i18n'
import { readinessErrorCopy } from '../../../../../core/errors/errorCatalog'
import {
  fetchCanvasBillingReadiness,
  fetchCanvasBillingReadinessMatrix,
} from './canvasBillingReadinessResource'

export type CanvasBillingQuoteState = {
  service: Service
  status: 'idle' | 'loading' | 'ready' | 'blocked' | 'error'
  unitCredits?: number
  totalCredits?: number
  availableCredits?: number
  priceVersionId?: number
  isSufficient?: boolean
  reason?: string | undefined
}

type CanvasBillingReadinessFetcher = (
  service: Service,
  request?: Parameters<typeof fetchCanvasBillingReadiness>[1],
) => Promise<SubmitTaskReadinessData>
type CanvasBillingReadinessMatrixFetcher = (
  requests: NonNullable<CanvasBillingReadinessRequest>[],
) => Promise<SubmitTaskReadinessData[]>

type CanvasBillingReadinessRequest = Parameters<typeof fetchCanvasBillingReadiness>[1]

export type CanvasBillingReadinessStore = {
  ensureService: (
    service: Service,
    options?: { shouldForce?: boolean; request?: CanvasBillingReadinessRequest },
  ) => Promise<void>
  ensureMatrix: (
    entries: Array<{
      service: Service
      request: NonNullable<CanvasBillingReadinessRequest>
    }>,
  ) => Promise<void>
  getServiceState: (
    service: Service,
    request?: CanvasBillingReadinessRequest,
  ) => CanvasBillingQuoteState
}

const EMPTY_STATES = Object.fromEntries(
  SERVICE_SLUGS.map((service) => [service, { service, status: 'idle' as const }]),
) as Record<Service, CanvasBillingQuoteState>

export const canvasBillingReadinessKey: InjectionKey<CanvasBillingReadinessStore> = Symbol(
  'canvas-billing-readiness',
)

function isService(value: string | undefined): value is Service {
  return SERVICE_SLUGS.includes(value as Service)
}

function buildReadinessStateKey(service: Service, request?: CanvasBillingReadinessRequest): string {
  return request === undefined ? service : `${service}:${JSON.stringify(request)}`
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
  const isCustomFetcher = options.fetchReadiness !== undefined
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
    if (!isService(readiness.service)) return null
    const service = readiness.service
    const billing = readiness.billing
    if (billing && !billing.sufficient) {
      return {
        service,
        status: 'ready',
        unitCredits: billing.unit_credits,
        totalCredits: billing.total_credits,
        availableCredits: billing.available_credits,
        priceVersionId: billing.price_version_id,
        isSufficient: false,
        reason: i18n.global.t('billing.insufficientBalanceDetail', {
          required: formatIdeaAmount(billing.total_credits),
          available: formatIdeaAmount(billing.available_credits),
        }),
      }
    }
    if (!readiness.ready) {
      return {
        service,
        status: 'blocked',
        reason: readinessErrorCopy(readiness.error_code),
      }
    }
    if (!billing) {
      return {
        service,
        status: 'error',
        reason: i18n.global.t('common.errors.billingInfoUnavailable'),
      }
    }
    return {
      service,
      status: 'ready',
      unitCredits: billing.unit_credits,
      totalCredits: billing.total_credits,
      availableCredits: billing.available_credits,
      priceVersionId: billing.price_version_id,
      isSufficient: billing.sufficient,
    }
  }

  function applyReadinessForKey(key: string, readiness: SubmitTaskReadinessData): void {
    const state = resolveReadinessState(readiness)
    if (!state) return
    failedAt.delete(key)
    states.set(key, state)
    loadedAt.set(key, now())
  }

  async function ensureService(
    service: Service,
    requestOptions: {
      shouldForce?: boolean
      request?: CanvasBillingReadinessRequest
    } = {},
  ): Promise<void> {
    const stateKey = buildReadinessStateKey(service, requestOptions.request)
    const lastFailureAt = failedAt.get(stateKey)
    if (lastFailureAt !== undefined && now() - lastFailureAt < cacheMs) return
    const lastLoadedAt = loadedAt.get(stateKey) ?? 0
    if (!requestOptions.shouldForce && lastLoadedAt > 0 && now() - lastLoadedAt < cacheMs) return
    const existing = pending.get(stateKey)
    if (existing) return existing

    const previous = states.get(stateKey) ?? EMPTY_STATES[service]
    states.set(stateKey, { ...previous, status: 'loading', reason: undefined })
    const request = fetchReadiness(service, requestOptions.request)
      .then((readiness) => applyReadinessForKey(stateKey, readiness))
      .catch(() => {
        // Optional Billing/provider integrations can be intentionally unavailable.
        // Cache the failure briefly so reactive quote consumers do not create a request storm.
        failedAt.set(stateKey, now())
        states.set(stateKey, {
          service,
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
    entries: Array<{
      service: Service
      request: NonNullable<CanvasBillingReadinessRequest>
    }>,
  ): Promise<void> {
    // Custom fetchers are used by isolated consumers/tests that only implement the
    // single-service contract; preserve that adapter seam without issuing a real request.
    if (isCustomFetcher && options.fetchReadinessMatrix === undefined) {
      await Promise.all(entries.map(({ service, request }) => ensureService(service, { request })))
      return
    }
    const stale = entries.filter(({ service, request }) => {
      const key = buildReadinessStateKey(service, request)
      const loaded = loadedAt.get(key) ?? 0
      const failed = failedAt.get(key) ?? 0
      return now() - loaded >= cacheMs && now() - failed >= cacheMs
    })
    if (stale.length === 0) return
    const batchKey = stale
      .map(({ service, request }) => buildReadinessStateKey(service, request))
      .sort()
      .join('|')
    const pendingBatch = pendingMatrices.get(batchKey)
    if (pendingBatch) return pendingBatch
    const requests = stale.map((entry) => entry.request)
    const batch = fetchReadinessMatrix(requests)
      .then((values) => {
        values.forEach((readiness, index) => {
          const entry = stale[index]
          if (entry)
            applyReadinessForKey(buildReadinessStateKey(entry.service, entry.request), readiness)
        })
      })
      .catch(() => {
        const timestamp = now()
        stale.forEach(({ service, request }) => {
          const key = buildReadinessStateKey(service, request)
          failedAt.set(key, timestamp)
          states.set(key, {
            service,
            status: 'error',
            reason: i18n.global.t('common.errors.billingInfoUnavailable'),
          })
        })
      })
      .finally(() => pendingMatrices.delete(batchKey))
    pendingMatrices.set(batchKey, batch)
    return batch
  }

  return {
    ensureService,
    ensureMatrix,
    getServiceState: (service, request) =>
      states.get(buildReadinessStateKey(service, request)) ?? EMPTY_STATES[service],
  }
}

export function provideCanvasBillingReadiness(
  store: CanvasBillingReadinessStore = createCanvasBillingReadinessStore(),
): CanvasBillingReadinessStore {
  provide(canvasBillingReadinessKey, store)
  return store
}

export function useCanvasBillingReadinessStore(): CanvasBillingReadinessStore | null {
  return inject(canvasBillingReadinessKey, null)
}

export function useCanvasBillingQuote(
  service: () => Service,
  enabled: () => boolean = () => true,
  request: () => Parameters<typeof fetchCanvasBillingReadiness>[1] = () => undefined,
): ComputedRef<CanvasBillingQuoteState | null> {
  const store = useCanvasBillingReadinessStore()
  watchEffect(() => {
    if (!store || !enabled()) return
    const readinessRequest = request()
    void store.ensureService(service(), {
      request: readinessRequest,
      // Readiness quotes include quantity and resolution. Dynamic node controls
      // must bypass the service-level cache when they provide a concrete request.
      shouldForce: readinessRequest !== undefined,
    })
  })
  return computed(() => {
    if (!store) return null
    return store.getServiceState(service(), request())
  })
}
