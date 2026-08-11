import { effectScope, nextTick, reactive, ref } from 'vue'
import type { LocationQuery } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'
import { createAccountManagementOverlay } from './accountManagementOverlay'

describe('Account Management overlay', () => {
  it('owns account entry state and closes it when the current user leaves an allowed surface', async () => {
    const route = reactive({
      fullPath: '/user/boards',
      hash: '',
      path: '/user/boards',
      query: {} as LocationQuery,
    })
    const currentUser = ref<object | null>({ id: 'user-1' })
    const scope = effectScope()
    const overlay = scope.run(() =>
      createAccountManagementOverlay({
        currentUser,
        reportError: vi.fn(),
        navigation: {
          replace: vi.fn().mockResolvedValue(undefined),
          resolve: (location) => ({ fullPath: location.path }),
        },
        route,
      }),
    )!

    overlay.commands.open('billing')

    expect(overlay.viewModel.visible.value).toBe(true)
    expect(overlay.viewModel.initialTab.value).toBe('billing')

    route.path = '/user/login'
    route.fullPath = '/user/login'
    await nextTick()

    expect(overlay.viewModel.allowed.value).toBe(false)
    expect(overlay.viewModel.visible.value).toBe(false)

    scope.stop()
  })

  it('consumes a Checkout return once and keeps the overlay open through its URL cleanup', async () => {
    const publicToken = '0198cd29-f457-7e4b-b95e-6cfde41b7222'
    const route = reactive({
      fullPath: `/user/boards?boardId=board-8&account=recharge&payment_return=${publicToken}&checkout_return=success`,
      hash: '',
      path: '/user/boards',
      query: {
        account: 'recharge',
        boardId: 'board-8',
        checkout_return: 'success',
        payment_return: publicToken,
      } as LocationQuery,
    })
    const replace = vi.fn().mockResolvedValue(undefined)
    const scope = effectScope()
    const overlay = scope.run(() =>
      createAccountManagementOverlay({
        currentUser: ref<object | null>({ id: 'user-1' }),
        reportError: vi.fn(),
        navigation: {
          replace,
          resolve: () => ({ fullPath: '/user/boards?boardId=board-8' }),
        },
        route,
      }),
    )!
    await nextTick()

    expect(overlay.viewModel.visible.value).toBe(true)
    expect(overlay.viewModel.initialTab.value).toBe('recharge')
    expect(overlay.viewModel.paymentReturn.value).toEqual({
      publicToken,
      source: 'success',
    })
    expect(replace).toHaveBeenCalledOnce()
    expect(replace).toHaveBeenCalledWith({
      hash: '',
      path: '/user/boards',
      query: { boardId: 'board-8' },
    })

    route.query = { boardId: 'board-8' }
    route.fullPath = '/user/boards?boardId=board-8'
    await nextTick()

    expect(overlay.viewModel.visible.value).toBe(true)
    expect(replace).toHaveBeenCalledOnce()

    scope.stop()
  })

  it('does not let a scheduled Checkout cleanup replace a newer navigation', async () => {
    const publicToken = '0198cd29-f457-7e4b-b95e-6cfde41b7222'
    const route = reactive({
      fullPath: `/user/boards?account=recharge&payment_return=${publicToken}&checkout_return=success`,
      hash: '',
      path: '/user/boards',
      query: {
        account: 'recharge',
        checkout_return: 'success',
        payment_return: publicToken,
      } as LocationQuery,
    })
    const replace = vi.fn().mockResolvedValue(undefined)
    const scope = effectScope()
    const overlay = scope.run(() =>
      createAccountManagementOverlay({
        currentUser: ref<object | null>({ id: 'user-1' }),
        reportError: vi.fn(),
        navigation: {
          replace,
          resolve: () => ({ fullPath: '/user/boards' }),
        },
        route,
      }),
    )!

    route.path = '/user/canvas'
    route.query = { boardId: 'new-board' }
    route.fullPath = '/user/canvas?boardId=new-board'
    await nextTick()

    expect(replace).not.toHaveBeenCalled()
    expect(overlay.viewModel.visible.value).toBe(false)

    scope.stop()
  })
})
