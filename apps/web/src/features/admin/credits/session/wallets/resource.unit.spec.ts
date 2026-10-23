import { describe, expect, it, vi } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { waitFor } from '@/test/render'
import { notifyUserCreditsInvalidation } from '@/core/query/userCreditsInvalidation'
import { adminCreditsBillingApi, type AdminCreditsBillingApi } from '../shared/api'
import { useAdminWalletResource, type AdminWalletTarget } from './resource'

function setupResource(initialTarget: AdminWalletTarget | null = null) {
  const target = ref(initialTarget)
  const cursor = ref<string>()
  const api: AdminCreditsBillingApi = {
    ...adminCreditsBillingApi,
    getWallet: vi.fn().mockResolvedValue({ billing_account_id: 'ba_user' }),
    getAccountWallet: vi.fn().mockResolvedValue({ billing_account_id: 'ba_shared' }),
    listLedger: vi.fn().mockResolvedValue({ transactions: [], nextCursor: 'next' }),
    listAccountLedger: vi.fn().mockResolvedValue({ transactions: [] }),
    grant: vi.fn().mockResolvedValue({ transaction_id: 'txn_grant' }),
    deduct: vi.fn().mockResolvedValue({ transaction_id: 'txn_deduction' }),
    refund: vi.fn().mockResolvedValue({ transaction_id: 'txn_refund' }),
    correct: vi.fn().mockResolvedValue({ transaction_id: 'txn_correction' }),
    restrictWallet: vi.fn().mockResolvedValue({ restriction_id: 'restriction_one' }),
    releaseWalletRestriction: vi.fn().mockResolvedValue({}),
  }
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  let resource!: ReturnType<typeof useAdminWalletResource>
  const wrapper = mount(
    defineComponent({
      setup() {
        resource = useAdminWalletResource(target, cursor, { api })
        return () => h('div')
      },
    }),
    { global: { plugins: [[VueQueryPlugin, { queryClient }]] } },
  )
  return { target, cursor, api, queryClient, resource, wrapper }
}

describe('Admin wallet target lifecycle', () => {
  it('does not read without a target and selects the matching adapter and cache namespace', async () => {
    const { target, cursor, api, resource, queryClient, wrapper } = setupResource()
    expect(api.getWallet).not.toHaveBeenCalled()
    expect(api.getAccountWallet).not.toHaveBeenCalled()
    target.value = { kind: 'user', id: 'usr_one' }
    await waitFor(() => expect(api.getWallet).toHaveBeenCalledWith('usr_one', expect.anything()))
    cursor.value = 'cursor_one'
    await waitFor(() => expect(resource.ledgerDataCriteria.value?.cursor).toBe('cursor_one'))
    expect(api.listLedger).toHaveBeenLastCalledWith(
      'usr_one',
      expect.objectContaining({ cursor: 'cursor_one' }),
      expect.anything(),
    )
    target.value = { kind: 'billingAccount', id: 'ba_shared' }
    await waitFor(() => expect(resource.ledgerDataCriteria.value?.target).toEqual(target.value))
    expect(api.getAccountWallet).toHaveBeenCalledWith('ba_shared', expect.anything())
    expect(
      queryClient.getQueryData(['control', 'credits', 'wallets', 'usr_one', 'summary']),
    ).toBeDefined()
    expect(
      queryClient.getQueryData(['control', 'credits', 'accounts', 'ba_shared', 'summary']),
    ).toBeDefined()
    wrapper.unmount()
    queryClient.clear()
  })

  it('tears down user invalidation on account mode, disabled state, and disposal', async () => {
    const { target, api, queryClient, wrapper } = setupResource({ kind: 'user', id: 'usr_one' })
    await waitFor(() => expect(api.getWallet).toHaveBeenCalledTimes(1))
    const invalidation = vi.spyOn(queryClient, 'invalidateQueries')
    notifyUserCreditsInvalidation({ source: 'task-hold', taskId: 'task_one' })
    expect(invalidation).toHaveBeenCalledTimes(1)
    target.value = { kind: 'billingAccount', id: 'ba_shared' }
    notifyUserCreditsInvalidation({ source: 'task-hold', taskId: 'task_two' })
    expect(invalidation).toHaveBeenCalledTimes(1)
    target.value = null
    notifyUserCreditsInvalidation({ source: 'task-hold', taskId: 'task_three' })
    expect(invalidation).toHaveBeenCalledTimes(1)
    target.value = { kind: 'user', id: 'usr_one' }
    wrapper.unmount()
    notifyUserCreditsInvalidation({ source: 'task-hold', taskId: 'task_four' })
    expect(invalidation).toHaveBeenCalledTimes(1)
    queryClient.clear()
  })

  it('keeps runtime command validation, explicit targets, distinct command routes and idempotency', async () => {
    const { resource, api, wrapper, queryClient } = setupResource()
    await expect(resource.executeCommand({ kind: 'grant', amountCredits: 1 })).rejects.toThrow()
    await expect(
      resource.executeCommand({
        kind: 'grant',
        userPublicId: 'usr_one',
        billingAccountId: 'ba_shared',
        amountCredits: 1,
      }),
    ).rejects.toThrow()
    await expect(
      resource.executeCommand({
        kind: 'refund',
        billingAccountId: 'ba_shared',
        amountCredits: 2,
        chargeTransactionId: 'txn_charge',
        remainingRefundableCredits: 1,
      }),
    ).rejects.toThrow()
    for (const kind of ['grant', 'deduction', 'refund', 'correction'] as const) {
      await resource.executeCommand({
        kind,
        billingAccountId: 'ba_shared',
        amountCredits: 1,
        chargeTransactionId: 'txn_charge',
        remainingRefundableCredits: 2,
        originalTransactionId: 'txn_original',
        originalAmountCredits: 2,
      })
      const operation =
        kind === 'deduction' ? api.deduct : kind === 'correction' ? api.correct : api[kind]
      expect(operation).toHaveBeenCalledWith(
        expect.objectContaining({ billing_account_id: 'ba_shared', amount_credits: 1 }),
        { idempotencyKey: expect.stringContaining(`control-ui:${kind}:`) },
      )
    }
    await resource.restrictMutation.mutateAsync({
      body: {
        billing_account_id: 'ba_shared',
        type: 'spend',
        source: 'control_review',
        related_object_type: 'test',
        related_object_id: 'one',
      },
      idempotencyKey: 'restrict-one',
    })
    await resource.releaseMutation.mutateAsync({
      restrictionId: 'restriction_one',
      body: {},
      idempotencyKey: 'release-one',
    })
    expect(api.releaseWalletRestriction).toHaveBeenCalledWith(
      'restriction_one',
      {},
      { idempotencyKey: 'release-one' },
    )
    wrapper.unmount()
    queryClient.clear()
  })
})
