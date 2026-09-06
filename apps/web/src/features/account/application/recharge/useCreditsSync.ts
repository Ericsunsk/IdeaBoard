import { onScopeDispose } from 'vue'
import { useQueryClient, type QueryClient } from '@tanstack/vue-query'
import { subscribeUserCreditsInvalidation } from '../../../../shared/query/userCreditsInvalidation'
import { billingRecordsQueryKeys, creditsQueryKeys } from '../../infrastructure'

type CreditsSyncSubscription = {
  consumers: number
  unsubscribe: () => void
}

const syncSubscriptions = new WeakMap<QueryClient, CreditsSyncSubscription>()

export function useCreditsSync(): void {
  const queryClient = useQueryClient()
  let sync = syncSubscriptions.get(queryClient)

  if (!sync) {
    sync = {
      consumers: 0,
      unsubscribe: subscribeUserCreditsInvalidation(() => {
        void queryClient.invalidateQueries({ queryKey: creditsQueryKeys.all })
        void queryClient.invalidateQueries({ queryKey: billingRecordsQueryKeys.all })
      }),
    }
    syncSubscriptions.set(queryClient, sync)
  }

  sync.consumers += 1
  onScopeDispose(() => {
    if (!sync) return
    sync.consumers -= 1
    if (sync.consumers > 0) return
    sync.unsubscribe()
    syncSubscriptions.delete(queryClient)
  })
}
