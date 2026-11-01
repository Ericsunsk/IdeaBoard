import { computed, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { fetchCreditWallet, creditsQueryKeys } from './resource'
import { useBillingAccount } from '../billing-account/useBillingAccount'
import { useCreditsSync } from './useCreditsSync'
import { accountScoped } from '../../../../core/query/queryDefaults'

export type CreditWalletState =
  | { status: 'loading' }
  | { status: 'ready'; availableCredits: number; billingAccountId: string; isShared: boolean }
  | { status: 'error'; error: Error }

export function useCreditWallet(enabled: MaybeRefOrGetter<boolean> = true) {
  useCreditsSync()
  const billing = useBillingAccount(enabled)
  const query = useQuery({
    queryKey: computed(() =>
      creditsQueryKeys.wallet(
        billing.walletScope.value.workspaceId,
        billing.walletScope.value.billingAccountId,
        billing.walletScope.value.identity,
      ),
    ),
    queryFn: ({ queryKey, signal }) => fetchCreditWallet(queryKey[3], signal, queryKey[4]),
    enabled: billing.isReady,
    ...accountScoped(),
  })
  const readError = billing.readError(query.error)
  const state = computed<CreditWalletState>(() => {
    const error = readError.value
    if (error) {
      return {
        status: 'error',
        error,
      }
    }
    const data = query.data.value
    if (!billing.isReady.value || query.isPending.value || data == null) {
      return { status: 'loading' }
    }
    return {
      status: 'ready',
      availableCredits: data.available_credits,
      billingAccountId: data.billing_account_id,
      isShared: billing.isShared.value,
    }
  })

  return { state }
}
