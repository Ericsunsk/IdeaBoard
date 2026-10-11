import { QueryClient } from '@tanstack/vue-query'
import { describe, expect, it, vi } from 'vitest'
import { billingRecordsQueryKeys } from './billing/resource'
import { creditsQueryKeys } from './credits/resource'
import { invalidateUserMoney } from './invalidateUserMoney'

describe('invalidateUserMoney', () => {
  it('invalidates the credits and billing roots together, credits first', () => {
    const queryClient = new QueryClient()
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries').mockResolvedValue()

    invalidateUserMoney(queryClient)

    expect(invalidateQueries).toHaveBeenCalledTimes(2)
    expect(invalidateQueries).toHaveBeenNthCalledWith(1, { queryKey: creditsQueryKeys.all })
    expect(invalidateQueries).toHaveBeenNthCalledWith(2, {
      queryKey: billingRecordsQueryKeys.all,
    })

    queryClient.clear()
  })
})
