import { onScopeDispose } from 'vue'
import { useQueryClient, type QueryClient } from '@tanstack/vue-query'
import { subscribeUserCreditsInvalidation } from '../../shared/query/userCreditsInvalidation'
import { userCreditsQueryKeys } from './userCreditsResource'

type UserCreditsInvalidationBridge = {
  consumers: number
  unsubscribe: () => void
}

const bridges = new WeakMap<QueryClient, UserCreditsInvalidationBridge>()

export function useUserCreditsInvalidationBridge(): void {
  const queryClient = useQueryClient()
  let bridge = bridges.get(queryClient)

  if (!bridge) {
    bridge = {
      consumers: 0,
      unsubscribe: subscribeUserCreditsInvalidation(() => {
        void queryClient.invalidateQueries({ queryKey: userCreditsQueryKeys.all })
      }),
    }
    bridges.set(queryClient, bridge)
  }

  bridge.consumers += 1
  onScopeDispose(() => {
    if (!bridge) return
    bridge.consumers -= 1
    if (bridge.consumers > 0) return
    bridge.unsubscribe()
    bridges.delete(queryClient)
  })
}
