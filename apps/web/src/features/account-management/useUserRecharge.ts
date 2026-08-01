import { computed, ref, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { PaymentReturnContext } from './paymentReturnRoute'
import { userBillingRecordsQueryKeys } from './userBillingRecordsResource'
import { userCreditsQueryKeys } from './userCreditsResource'
import {
  userPaymentsQueryKeys,
  useUserPaymentsContext,
  type UserPaymentCheckoutRequest,
  type UserPendingPaymentOrder,
  type UserPendingPaymentOrderList,
} from './userPaymentsResource'

const historyPageSize = 20
type PendingOrderAction = { publicToken: string; kind: 'continue' | 'abandon' }

export function useUserRecharge(
  active: MaybeRefOrGetter<boolean>,
  paymentReturn: MaybeRefOrGetter<PaymentReturnContext | null>,
) {
  const queryClient = useQueryClient()
  const { resource, navigateToCheckout } = useUserPaymentsContext()
  let checkoutIntent: { fingerprint: string; idempotencyKey: string } | null = null

  const optionsQuery = useQuery({
    queryKey: userPaymentsQueryKeys.options,
    queryFn: ({ signal }) => resource.getPurchaseOptions(signal),
    enabled: () => toValue(active),
    retry: false,
  })

  const pendingOrdersQuery = useQuery({
    queryKey: userPaymentsQueryKeys.pending,
    queryFn: ({ signal }) => resource.listPendingOrders(signal),
    enabled: () => toValue(active),
    retry: false,
  })

  const historyQuery = useInfiniteQuery({
    queryKey: userPaymentsQueryKeys.history,
    queryFn: ({ pageParam, signal }: { pageParam: string; signal: AbortSignal }) =>
      resource.listRecharges(
        {
          limit: historyPageSize,
          ...(pageParam ? { cursor: pageParam } : {}),
        },
        signal,
      ),
    initialPageParam: '',
    getNextPageParam: (lastPage) => lastPage.next_cursor,
    enabled: () => toValue(active),
    retry: false,
  })

  const returnToken = computed(() => toValue(paymentReturn)?.publicToken ?? '')
  const returnQuery = useQuery({
    queryKey: computed(() => userPaymentsQueryKeys.returnStatus(returnToken.value)),
    queryFn: ({ signal }) => resource.getReturnStatus(returnToken.value, signal),
    enabled: () => toValue(active) && returnToken.value.length > 0,
    retry: false,
  })

  const checkoutMutation = useMutation({
    mutationFn: (request: UserPaymentCheckoutRequest) => resource.createCheckout(request),
    onSuccess: (checkout) => navigateToCheckout(checkout.checkout_url),
  })

  const pendingActionTarget = ref<PendingOrderAction | null>(null)
  const pendingActionError = ref<unknown>(null)

  function updateCachedPendingOrder(
    publicToken: string,
    update: (order: UserPendingPaymentOrder) => UserPendingPaymentOrder,
  ) {
    queryClient.setQueryData<UserPendingPaymentOrderList>(userPaymentsQueryKeys.pending, (current) => {
      if (!current) return current
      return {
        ...current,
        items: current.items.map((item) => (item.public_token === publicToken ? update(item) : item)),
      }
    })
  }

  const pendingActionMutation = useMutation({
    mutationFn: (action: PendingOrderAction) =>
      action.kind === 'continue'
        ? resource.continuePendingOrder(action.publicToken)
        : resource.abandonPendingOrder(action.publicToken),
    onSuccess: (order, action) => {
      if (order.status !== 'checkout_open') {
        updateCachedPendingOrder(order.public_token, () => order)
      }
      void queryClient.invalidateQueries({ queryKey: userPaymentsQueryKeys.history })
      void queryClient.invalidateQueries({ queryKey: userCreditsQueryKeys.all })
      void queryClient.invalidateQueries({ queryKey: userBillingRecordsQueryKeys.all })
      if (action.kind === 'continue' && order.checkout_url) navigateToCheckout(order.checkout_url)
    },
  })

  function createCheckout(ideaQuantity: number) {
    const options = optionsQuery.data.value
    if (!options) return Promise.reject(new Error('充值选项尚未加载完成'))

    const fingerprint = [ideaQuantity, options.terms.version, options.refund_policy.version].join(
      ':',
    )
    if (checkoutIntent?.fingerprint !== fingerprint) {
      checkoutIntent = { fingerprint, idempotencyKey: globalThis.crypto.randomUUID() }
    }

    return checkoutMutation.mutateAsync({
      idea_quantity: ideaQuantity,
      idempotency_key: checkoutIntent.idempotencyKey,
      terms_version: options.terms.version,
      refund_policy_version: options.refund_policy.version,
      terms_accepted: true,
    })
  }

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

  watch(
    () => toValue(active),
    (enabled, _previous, onCleanup) => {
      if (!enabled) return
      const unsubscribe = resource.subscribe((event) => {
        if (event.status === 'failed' && event.public_token) {
          updateCachedPendingOrder(event.public_token, (order) => {
            const { checkout_url: _checkoutURL, ...withoutCheckoutURL } = order
            return { ...withoutCheckoutURL, status: 'payment_failed' }
          })
        } else {
          void queryClient.invalidateQueries({ queryKey: userPaymentsQueryKeys.pending })
        }
        void queryClient.invalidateQueries({ queryKey: userPaymentsQueryKeys.history })
        void queryClient.invalidateQueries({ queryKey: userCreditsQueryKeys.all })
        void queryClient.invalidateQueries({ queryKey: userBillingRecordsQueryKeys.all })
        if (event.public_token) {
          void queryClient.invalidateQueries({
            queryKey: userPaymentsQueryKeys.returnStatus(event.public_token),
          })
        }
      })
      onCleanup(unsubscribe)
    },
    { immediate: true },
  )

  const historyItems = computed(
    () => historyQuery.data.value?.pages.flatMap((page) => page.items) ?? [],
  )
  const hasLoadedHistory = computed(() => historyQuery.data.value !== undefined)

  return {
    purchaseOptions: optionsQuery.data,
    optionsPending: optionsQuery.isPending,
    optionsError: optionsQuery.error,
    retryOptions: optionsQuery.refetch,
    createCheckout,
    checkoutPending: checkoutMutation.isPending,
    checkoutError: checkoutMutation.error,
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
    historyItems,
    historyInitialPending: computed(() => historyQuery.isPending.value && !hasLoadedHistory.value),
    historyInitialError: computed(() => (hasLoadedHistory.value ? null : historyQuery.error.value)),
    historyLoadMoreError: computed(() =>
      historyQuery.isFetchNextPageError.value ? historyQuery.error.value : null,
    ),
    historyHasNextPage: historyQuery.hasNextPage,
    historyFetchingNextPage: historyQuery.isFetchingNextPage,
    retryHistory: historyQuery.refetch,
    loadMoreHistory: historyQuery.fetchNextPage,
  }
}
