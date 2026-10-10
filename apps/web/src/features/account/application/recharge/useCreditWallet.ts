import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { fetchCreditWallet, creditsQueryKeys } from '../../infrastructure'
import { i18n } from '../../../../core/i18n'
import { useAccountWorkspace } from '../accountWorkspace'
import { useCreditsSync } from './useCreditsSync'
import { accountScoped } from '../../../../core/query/queryDefaults'

export type CreditWalletState =
  | { status: 'loading' }
  | { status: 'ready'; availableCredits: number }
  | { status: 'error'; error: Error }

export function useCreditWallet(enabled: MaybeRefOrGetter<boolean> = true) {
  useCreditsSync()
  const { workspaceId, selectionSettled } = useAccountWorkspace({ enabled })
  const query = useQuery({
    queryKey: computed(() => creditsQueryKeys.wallet(workspaceId.value ?? '')),
    queryFn: ({ signal }) => fetchCreditWallet(workspaceId.value ?? '', signal),
    enabled: () => toValue(enabled) && selectionSettled.value,
    ...accountScoped(),
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
