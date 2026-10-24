import { QueryClient, type QueryFilters } from '@tanstack/vue-query'
import { describe, expect, it, vi } from 'vitest'
import { billingRecordsQueryKeys } from './billing/resource'
import { creditsQueryKeys } from './credits/resource'
import { usageQueryKeys } from './usage/resource'
import { invalidateUserMoney } from './invalidateUserMoney'

describe('invalidateUserMoney', () => {
  it('invalidates the credits, billing, and usage roots together, credits first', () => {
    const queryClient = new QueryClient()
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries').mockResolvedValue()

    invalidateUserMoney(queryClient)

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

    queryClient.clear()
  })
})
