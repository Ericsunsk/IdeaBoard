import { QueryClient, VueQueryPlugin, type QueryFilters } from '@tanstack/vue-query'
import { createApp, defineComponent } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { notifyUserCreditsInvalidation } from '../../../../core/query/userCreditsInvalidation'
import { useCreditsSync } from './useCreditsSync'
import { billingRecordsQueryKeys } from '../billing/resource'
import { creditsQueryKeys } from '../credits/resource'
import { usageQueryKeys } from '../usage/resource'

describe('useCreditsSync', () => {
  it('shares one settlement listener per QueryClient and releases it with the scope', () => {
    const queryClient = new QueryClient()
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries').mockResolvedValue()
    const host = document.createElement('div')
    document.body.appendChild(host)
    const app = createApp(
      defineComponent({
        setup() {
          useCreditsSync()
          useCreditsSync()
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

    expect(invalidateQueries).toHaveBeenCalledTimes(3)
    expect(invalidateQueries).toHaveBeenNthCalledWith(1, {
      queryKey: creditsQueryKeys.all,
      exact: false,
    } satisfies QueryFilters)
    expect(invalidateQueries).toHaveBeenNthCalledWith(2, {
      queryKey: billingRecordsQueryKeys.all,
      exact: false,
    } satisfies QueryFilters)
    expect(invalidateQueries).toHaveBeenNthCalledWith(3, {
      queryKey: usageQueryKeys.all,
      exact: false,
    } satisfies QueryFilters)

    app.unmount()
    notifyUserCreditsInvalidation({
      source: 'task-settlement',
      taskId: 'task-8',
      status: 'failed',
    })
    expect(invalidateQueries).toHaveBeenCalledTimes(3)

    queryClient.clear()
    host.remove()
  })
})
