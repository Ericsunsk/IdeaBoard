import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { createApp, defineComponent } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { notifyUserCreditsInvalidation } from '../../shared/query/userCreditsInvalidation'
import { useUserCreditsInvalidationBridge } from './useUserCreditsInvalidationBridge'
import { userBillingRecordsQueryKeys } from './userBillingRecordsResource'
import { userCreditsQueryKeys } from './userCreditsResource'

describe('useUserCreditsInvalidationBridge', () => {
  it('shares one settlement listener per QueryClient and releases it with the scope', () => {
    const queryClient = new QueryClient()
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries').mockResolvedValue()
    const host = document.createElement('div')
    document.body.appendChild(host)
    const app = createApp(
      defineComponent({
        setup() {
          useUserCreditsInvalidationBridge()
          useUserCreditsInvalidationBridge()
          return () => null
        },
      }),
    )
    app.use(VueQueryPlugin, { queryClient })
    app.mount(host)

    notifyUserCreditsInvalidation({
      source: 'task-settlement',
      taskId: 'task-7',
      status: 'completed',
    })

    expect(invalidateQueries).toHaveBeenCalledTimes(2)
    expect(invalidateQueries).toHaveBeenNthCalledWith(1, { queryKey: userCreditsQueryKeys.all })
    expect(invalidateQueries).toHaveBeenNthCalledWith(2, {
      queryKey: userBillingRecordsQueryKeys.all,
    })

    app.unmount()
    notifyUserCreditsInvalidation({
      source: 'task-settlement',
      taskId: 'task-8',
      status: 'failed',
    })
    expect(invalidateQueries).toHaveBeenCalledTimes(2)

    queryClient.clear()
    host.remove()
  })
})
