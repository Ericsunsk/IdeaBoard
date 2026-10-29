import { computed, ref, toValue, watch, type MaybeRefOrGetter, type Ref } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { PaymentReturnContext } from '../../core/return'
import { i18n } from '../../../../core/i18n'
import { createUniqueId } from '../../../../core/utils/ids'
import {
  paymentsQueryKeys,
  usePaymentsContext,
  type PaymentCheckoutRequest,
  type PaymentEvent,
  type PaymentPurchaseOptions,
  type PendingPaymentOrder,
  type PendingPaymentOrderList,
} from './resource'
import { invalidateUserMoney } from '../invalidateUserMoney'
import { useAccountWorkspace } from '../accountWorkspace'
import { useCursorInfiniteList } from '../../../../core/query/useCursorInfiniteList'
import { noRetry } from '../../../../core/query/queryDefaults'

type PendingOrderAction = { publicToken: string; kind: 'continue' | 'abandon' }

export function useRecharge(
  active: MaybeRefOrGetter<boolean>,
  paymentReturn: MaybeRefOrGetter<PaymentReturnContext | null>,
) {
  const queryClient = useQueryClient()
  const { resource, navigateToCheckout } = usePaymentsContext()
  const { workspaceId, isSelectionSettled } = useAccountWorkspace()
  const activeWorkspaceId = computed(() => workspaceId.value ?? '')
  const transportReady = computed(() => toValue(active) && isSelectionSettled.value)

  const returnToken = computed(() => toValue(paymentReturn)?.publicToken ?? '')
  const { optionsQuery, pendingOrdersQuery, history, returnQuery } = useRechargeQueries(
    resource,
    activeWorkspaceId,
    transportReady,
    returnToken,
  )

  const checkout = useCheckoutAction(resource, activeWorkspaceId, navigateToCheckout, optionsQuery)

  const pendingOrders = createPendingOrderActions(
    queryClient,
    resource,
    activeWorkspaceId,
    navigateToCheckout,
  )
  const { pendingActionTarget, pendingActionError, pendingActionMutation, runPendingOrderAction } =
    pendingOrders

  usePaymentEventSubscription({
    queryClient,
    resource,
    active,
    isSelectionSettled,
    activeWorkspaceId,
    updateCachedPendingOrder: pendingOrders.updateCachedPendingOrder,
    removeCachedPendingOrder: pendingOrders.removeCachedPendingOrder,
  })

  const historyQuery = history.query

  return {
    purchaseOptions: optionsQuery.data,
    optionsPending: optionsQuery.isPending,
    optionsError: optionsQuery.error,
    retryOptions: optionsQuery.refetch,
    createCheckout: checkout.createCheckout,
    checkoutPending: checkout.checkoutPending,
    checkoutError: checkout.checkoutError,
    pendingOrders: computed(() => pendingOrdersQuery.data.value?.items ?? []),
    pendingOrdersPending: pendingOrdersQuery.isPending,
    pendingOrdersError: pendingOrdersQuery.error,
    retryPendingOrders: pendingOrdersQuery.refetch,
    continuePendingOrder: (publicToken: string) =>
      runPendingOrderAction({ publicToken, kind: 'continue' }),
    abandonPendingOrder: (publicToken: string) =>
      runPendingOrderAction({ publicToken, kind: 'abandon' }),
    pendingOrderActionPending: pendingActionMutation.isPending,
    pendingOrderActionTarget: pendingActionTarget,
    pendingOrderActionError: pendingActionError,
    returnStatus: returnQuery.data,
    returnPending: returnQuery.isPending,
    returnError: returnQuery.error,
    retryReturn: returnQuery.refetch,
    historyItems: history.items,
    historyInitialPending: computed(
      () => historyQuery.isPending.value && !history.hasLoadedPage.value,
    ),
    historyInitialError: computed(() =>
      history.hasLoadedPage.value ? null : historyQuery.error.value,
    ),
    historyLoadMoreError: history.loadMoreError,
    historyHasNextPage: history.hasNextPage,
    historyFetchingNextPage: history.isFetchingNextPage,
    retryHistory: history.retryInitial,
    loadMoreHistory: history.loadMore,
  }
}

type PendingOrderActionContext = {
  queryClient: ReturnType<typeof useQueryClient>
  resource: ReturnType<typeof usePaymentsContext>['resource']
  workspaceId: MaybeRefOrGetter<string>
  navigateToCheckout: (checkoutURL: string) => void
}

function createPendingOrderActions(
  queryClient: PendingOrderActionContext['queryClient'],
  resource: PendingOrderActionContext['resource'],
  workspaceId: PendingOrderActionContext['workspaceId'],
  navigateToCheckout: PendingOrderActionContext['navigateToCheckout'],
) {
  const pendingActionTarget = ref<PendingOrderAction | null>(null)
  const pendingActionError = ref<unknown>(null)

  function updateCachedPendingOrder(
    publicToken: string,
    update: (order: PendingPaymentOrder) => PendingPaymentOrder,
  ) {
    queryClient.setQueryData<PendingPaymentOrderList>(
      paymentsQueryKeys.pending(toValue(workspaceId) ?? ''),
      (current) => {
        if (!current) return current
        return {
          ...current,
          items: current.items.map((item) =>
            item.public_token === publicToken ? update(item) : item,
          ),
        }
      },
    )
  }

  function removeCachedPendingOrder(publicToken: string) {
    queryClient.setQueryData<PendingPaymentOrderList>(
      paymentsQueryKeys.pending(toValue(workspaceId) ?? ''),
      (current) =>
        current
          ? { ...current, items: current.items.filter((item) => item.public_token !== publicToken) }
          : current,
    )
  }

  const pendingActionMutation = useMutation({
    mutationFn: (action: PendingOrderAction) =>
      action.kind === 'continue'
        ? resource.continuePendingOrder(toValue(workspaceId) ?? '', action.publicToken)
        : resource.abandonPendingOrder(toValue(workspaceId) ?? '', action.publicToken),
    onSuccess: (order, action) => {
      if (isPendingPaymentOrderStatus(order.status)) {
        updateCachedPendingOrder(order.public_token, () => order)
      } else {
        removeCachedPendingOrder(order.public_token)
      }
      // `setQueryData` above is a no-op when the pending list was never
      // fetched, so always stale-mark it: the SSE path invalidates
      // pending+history together and this mutation must match it.
      void queryClient.invalidateQueries({
        queryKey: paymentsQueryKeys.pending(toValue(workspaceId) ?? ''),
        exact: true,
      })
      void queryClient.invalidateQueries({
        queryKey: paymentsQueryKeys.history(toValue(workspaceId) ?? ''),
        exact: true,
      })
      invalidateUserMoney(queryClient)
      if (action.kind === 'continue' && order.checkout_url) navigateToCheckout(order.checkout_url)
    },
  })

  async function runPendingOrderAction(action: PendingOrderAction) {
    pendingActionTarget.value = action
    pendingActionError.value = null
    try {
      await pendingActionMutation.mutateAsync(action)
    } catch (error) {
      pendingActionError.value = error
      throw error
    }
  }

  return {
    pendingActionTarget,
    pendingActionError,
    pendingActionMutation,
    runPendingOrderAction,
    updateCachedPendingOrder,
    removeCachedPendingOrder,
  }
}

function useRechargeQueries(
  resource: PendingOrderActionContext['resource'],
  workspaceId: PendingOrderActionContext['workspaceId'],
  transportReady: MaybeRefOrGetter<boolean>,
  returnToken: MaybeRefOrGetter<string>,
) {
  const optionsQuery = useQuery({
    queryKey: computed(() => paymentsQueryKeys.options(toValue(workspaceId) ?? '')),
    queryFn: ({ signal }) => resource.getPurchaseOptions(toValue(workspaceId) ?? '', signal),
    enabled: transportReady,
    ...noRetry(),
  })

  const pendingOrdersQuery = useQuery({
    queryKey: computed(() => paymentsQueryKeys.pending(toValue(workspaceId) ?? '')),
    queryFn: ({ signal }) => resource.listPendingOrders(toValue(workspaceId) ?? '', signal),
    enabled: transportReady,
    ...noRetry(),
  })

  const history = useCursorInfiniteList({
    queryKey: computed(() => paymentsQueryKeys.history(toValue(workspaceId) ?? '')),
    fetchPage: ({ pageParam, signal, pageSize }) =>
      resource.listRecharges(
        toValue(workspaceId) ?? '',
        {
          limit: pageSize,
          ...(pageParam ? { cursor: pageParam } : {}),
        },
        signal,
      ),
    isEnabled: transportReady,
    getItems: (page) => page.items,
  })

  const returnQuery = useQuery({
    queryKey: computed(() =>
      paymentsQueryKeys.returnStatus(toValue(workspaceId) ?? '', toValue(returnToken)),
    ),
    queryFn: ({ signal }) =>
      resource.getReturnStatus(toValue(workspaceId) ?? '', toValue(returnToken), signal),
    enabled: computed(() => toValue(transportReady) && toValue(returnToken).length > 0),
    ...noRetry(),
  })

  return { optionsQuery, pendingOrdersQuery, history, returnQuery }
}

function useCheckoutAction(
  resource: PendingOrderActionContext['resource'],
  workspaceId: PendingOrderActionContext['workspaceId'],
  navigateToCheckout: PendingOrderActionContext['navigateToCheckout'],
  optionsQuery: { data: Ref<PaymentPurchaseOptions | undefined> },
) {
  let checkoutIntent: { fingerprint: string; idempotencyKey: string } | null = null

  const checkoutMutation = useMutation({
    mutationFn: (request: PaymentCheckoutRequest) =>
      resource.createCheckout(toValue(workspaceId) ?? '', request),
    onSuccess: (checkout) => {
      checkoutIntent = null
      navigateToCheckout(checkout.checkout_url)
    },
  })

  async function createCheckout(ideaQuantity: number) {
    const options = optionsQuery.data.value
    if (!options) return Promise.reject(new Error(i18n.global.t('account.errors.optionsNotReady')))

    const fingerprint = [ideaQuantity, options.terms.version, options.refund_policy.version].join(
      ':',
    )
    if (checkoutIntent?.fingerprint !== fingerprint) {
      checkoutIntent = { fingerprint, idempotencyKey: createUniqueId() }
    }

    const request: PaymentCheckoutRequest = {
      idea_quantity: ideaQuantity,
      idempotency_key: checkoutIntent.idempotencyKey,
      terms_version: options.terms.version,
      refund_policy_version: options.refund_policy.version,
      terms_accepted: true,
    }
    try {
      return await checkoutMutation.mutateAsync(request)
    } catch (error) {
      if (
        shouldRotateCheckoutIntent(error) &&
        checkoutIntent?.idempotencyKey === request.idempotency_key
      ) {
        checkoutIntent = null
      }
      throw error
    }
  }

  return {
    createCheckout,
    checkoutPending: checkoutMutation.isPending,
    checkoutError: checkoutMutation.error,
  }
}

function usePaymentEventSubscription(options: {
  queryClient: PendingOrderActionContext['queryClient']
  resource: PendingOrderActionContext['resource']
  active: MaybeRefOrGetter<boolean>
  isSelectionSettled: { value: boolean }
  activeWorkspaceId: PendingOrderActionContext['workspaceId']
  updateCachedPendingOrder: PendingOrderCache['updateCachedPendingOrder']
  removeCachedPendingOrder: PendingOrderCache['removeCachedPendingOrder']
}) {
  const { queryClient, resource, active, isSelectionSettled, activeWorkspaceId } = options
  watch(
    () => [toValue(active), isSelectionSettled.value, toValue(activeWorkspaceId) ?? ''] as const,
    ([enabled, settled, settledWorkspaceId], _previous, onCleanup) => {
      if (!enabled || !settled || !settledWorkspaceId) return
      const unsubscribe = resource.subscribe(settledWorkspaceId, (event) =>
        handlePaymentEvent(queryClient, settledWorkspaceId, event, {
          updateCachedPendingOrder: options.updateCachedPendingOrder,
          removeCachedPendingOrder: options.removeCachedPendingOrder,
        }),
      )
      onCleanup(unsubscribe)
    },
    { immediate: true },
  )
}

function isPendingPaymentOrderStatus(status: PendingPaymentOrder['status']): boolean {
  return (
    status === 'checkout_open' ||
    status === 'async_payment_pending' ||
    status === 'paid_pending_credit'
  )
}

type PendingOrderCache = {
  updateCachedPendingOrder: (
    publicToken: string,
    update: (order: PendingPaymentOrder) => PendingPaymentOrder,
  ) => void
  removeCachedPendingOrder: (publicToken: string) => void
}

function handlePaymentEvent(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceId: string,
  event: PaymentEvent,
  cache: PendingOrderCache,
) {
  if (event.status === 'failed' && event.public_token) {
    cache.updateCachedPendingOrder(event.public_token, (order) => {
      const { checkout_url: _checkoutURL, ...withoutCheckoutURL } = order
      return { ...withoutCheckoutURL, status: 'payment_failed' }
    })
  } else {
    void queryClient.invalidateQueries({
      queryKey: paymentsQueryKeys.pending(workspaceId),
      exact: true,
    })
  }
  void queryClient.invalidateQueries({
    queryKey: paymentsQueryKeys.history(workspaceId),
    exact: true,
  })
  invalidateUserMoney(queryClient)
  if (event.public_token) {
    void queryClient.invalidateQueries({
      queryKey: paymentsQueryKeys.returnStatus(workspaceId, event.public_token),
      exact: true,
    })
  }
}

function shouldRotateCheckoutIntent(error: unknown): boolean {
  if (typeof error !== 'object' || error === null || !('code' in error)) return false
  const code = (error as { code?: unknown }).code
  return (
    code === 'checkout_request_rejected' ||
    code === 'checkout_session_expired' ||
    code === 'payment_already_completed' ||
    code === 'billing_checkout_conflict'
  )
}
