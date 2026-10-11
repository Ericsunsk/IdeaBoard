import { onScopeDispose } from 'vue'
import { useQueryClient, type QueryClient } from '@tanstack/vue-query'
import { subscribeUserCreditsInvalidation } from '../../../../core/query/userCreditsInvalidation'
import { invalidateUserMoney } from '../invalidateUserMoney'

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
        invalidateUserMoney(queryClient)
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
