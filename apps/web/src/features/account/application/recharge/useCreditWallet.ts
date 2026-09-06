import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { fetchCreditWallet, creditsQueryKeys } from '../../infrastructure'
import { i18n } from '../../../../shared/i18n'
import { useCreditsSync } from './useCreditsSync'

export type CreditWalletState =
  | { status: 'loading' }
  | { status: 'ready'; availableCredits: number }
  | { status: 'error'; error: Error }

export function useCreditWallet(enabled: MaybeRefOrGetter<boolean> = true) {
  useCreditsSync()
  const query = useQuery({
    queryKey: creditsQueryKeys.wallet,
    queryFn: fetchCreditWallet,
    enabled: () => toValue(enabled),
    staleTime: 15_000,
  })
  const state = computed<CreditWalletState>(() => {
    if (query.isError.value) {
      return {
        status: 'error',
        error: query.error.value ?? new Error(i18n.global.t('account.errors.walletLoadFailed')),
      }
    }
    const availableCredits = query.data.value?.available_credits
    if (query.isPending.value || availableCredits == null) {
      return { status: 'loading' }
    }
    return { status: 'ready', availableCredits }
  })

  return { state }
}
