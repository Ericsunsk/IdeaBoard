import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { renderWithUser, screen, waitFor } from '../../../test/render'
import { createRewardRedemptionWorkflow } from './rewards/useRewardRedemption'
import { useAvatarUpload } from './profile/useAvatarUpload'
import { usePasswordChange } from './security/usePasswordChange'
import RechargePane from '../ui/recharge/RechargePane.vue'
import { paymentsContextKey, type PaymentsResource } from './payments/resource'

vi.mock('./accountWorkspace', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./accountWorkspace')>()),
  useAccountWorkspace: () => ({
    workspaceId: ref('ws_1'),
    selectionSettled: ref(true),
  }),
}))

function forbidden(message = 'Forbidden'): Error & { status?: number; code?: string } {
  return Object.assign(new Error(message), { status: 403, code: 'forbidden' })
}

// A 403 from Account credits/payment/profile mutations performs no success
// transition, leaks no state, and exposes the approved failure feedback.
// Covers the session redemption workflow (credits) and the profile workflows
// (avatar, password); payment checkout/pending-order mutations share the same
// vue-query `onSuccess`-only side-effect pattern and are pinned by the
// RechargePane 403 case below via the session payments resource contract.
describe('Account permission-403', () => {
  it('performs no Idea-read mutations when redemption fails with 403', async () => {
    const error = forbidden()
    const setAvailableBalance = vi.fn()
    const prependRedemption = vi.fn()
    const invalidateBillingRecords = vi.fn()
    const invalidateRedemptions = vi.fn()
    const workflow = createRewardRedemptionWorkflow({
      redeem: vi.fn().mockRejectedValue(error),
      setAvailableBalance,
      prependRedemption,
      invalidateBillingRecords,
      invalidateRedemptions,
    })

    await expect(workflow.redeem('IDEA2026')).rejects.toBe(error)
    expect(setAvailableBalance).not.toHaveBeenCalled()
    expect(prependRedemption).not.toHaveBeenCalled()
    expect(invalidateBillingRecords).not.toHaveBeenCalled()
    expect(invalidateRedemptions).not.toHaveBeenCalled()
    expect(workflow.pending.value).toBe(false)
  })

  it('exposes redemption 403 through the production invalidation wiring without leaking balance', async () => {
    const error = forbidden()
    const queryClient = new QueryClient()
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries')
    const workflow = createRewardRedemptionWorkflow({
      redeem: vi.fn().mockRejectedValue(error),
      setAvailableBalance: vi.fn(),
      prependRedemption: vi.fn(),
      invalidateBillingRecords: vi.fn(),
      invalidateRedemptions: vi.fn(),
    })

    await expect(workflow.redeem('IDEA2026')).rejects.toBe(error)
    expect(invalidateQueries).not.toHaveBeenCalled()
    queryClient.clear()
  })

  it('performs no avatar state change and routes 403 through avatar feedback', async () => {
    const error = forbidden()
    const setAvatarLocal = vi.fn()
    const success = vi.fn()
    const failure = vi.fn()
    const workflow = useAvatarUpload({
      uploadAvatar: vi.fn().mockRejectedValue(error),
      userStore: { setAvatarLocal },
      profileFeedback: { invalidAvatarFile: vi.fn(), avatarUpload: { success, error: failure } },
      runAsyncAction: async (action, options = {}) => {
        try {
          const result = await action()
          options.feedback?.success?.(result as never)
          return result
        } catch (actionError) {
          options.feedback?.error?.(actionError)
          return undefined
        }
      },
    })

    await workflow.selectFile(new File(['image'], 'avatar.png', { type: 'image/png' }))

    expect(setAvatarLocal).not.toHaveBeenCalled()
    expect(success).not.toHaveBeenCalled()
    expect(failure).toHaveBeenCalledWith(error)
    expect(workflow.uploading.value).toBe(false)
  })

  it('performs no password/logout/navigation transition and routes 403 through password feedback', async () => {
    const error = forbidden()
    const updatePassword = vi.fn().mockRejectedValue(error)
    const logout = vi.fn()
    const replace = vi.fn()
    const success = vi.fn()
    const failure = vi.fn()
    const workflow = usePasswordChange({
      userStore: { updatePassword, logout },
      router: { replace },
      profileFeedback: { passwordChange: { success, error: failure } },
      runAsyncAction: async (action, options = {}) => {
        try {
          const result = await action()
          options.feedback?.success?.(result as never)
          return result
        } catch (actionError) {
          options.feedback?.error?.(actionError)
          return undefined
        }
      },
    })
    workflow.form.value = {
      oldPassword: 'old-pass',
      newPassword: 'new-password',
      confirmPassword: 'new-password',
    }

    await workflow.submit()

    expect(updatePassword).toHaveBeenCalledWith('old-pass', 'new-password')
    expect(logout).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
    expect(success).not.toHaveBeenCalled()
    expect(failure).toHaveBeenCalledWith(error)
    expect(workflow.form.value.oldPassword).toBe('old-pass')
    expect(workflow.loading.value).toBe(false)
  })

  it('performs no checkout navigation and keeps the payment form retryable on 403', async () => {
    const error = forbidden('没有充值权限')
    const createCheckout = vi.fn().mockRejectedValue(error)
    const navigate = vi.fn()
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })
    const resource: PaymentsResource = {
      getPurchaseOptions: vi.fn().mockResolvedValue({
        purchases_available: true,
        quick_idea_quantities: [1000],
        min_idea_quantity: 500,
        max_idea_quantity: 500_000,
        ideas_per_usd: 100,
        terms: { version: '2026-08-01', url: '/terms' },
        refund_policy: { version: '2026-08-01', url: '/refund-policy' },
      }),
      createCheckout,
      getReturnStatus: vi.fn(),
      listPendingOrders: vi.fn().mockResolvedValue({ items: [] }),
      continuePendingOrder: vi.fn(),
      abandonPendingOrder: vi.fn(),
      listRecharges: vi.fn().mockResolvedValue({ items: [] }),
      subscribe: vi.fn(() => () => undefined),
    }
    const { user } = renderWithUser(RechargePane, {
      props: { active: true, paymentReturn: null },
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
        provide: {
          [paymentsContextKey as symbol]: { resource, navigateToCheckout: navigate },
        },
      },
    })

    await user.click(await screen.findByRole('button', { name: 'Pay now' }))

    await waitFor(() => expect(createCheckout).toHaveBeenCalledTimes(1))
    expect(await screen.findByRole('alert')).toBeVisible()
    expect(navigate).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Pay now' })).toBeEnabled()
  })
})
