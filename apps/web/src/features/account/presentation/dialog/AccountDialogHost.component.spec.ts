import { createRouter, createMemoryHistory } from 'vue-router'
import type { Router, RouteRecordRaw } from 'vue-router'
import { computed, defineComponent, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '../../../../test/render'
import { useUserStore } from '../../../auth'
import type { User } from '../../../../shared/api/sessionUser'
import {
  createAccountOverlay,
  provideAccountOverlay,
  type AccountOverlay,
} from '../../application/overlay'
import AccountDialogHost from './AccountDialogHost.vue'

const routes: RouteRecordRaw[] = [
  { path: '/user/boards', component: { template: '<div />' } },
  { path: '/user/canvas', component: { template: '<div />' } },
  { path: '/user/login', component: { template: '<div />' } },
  { path: '/user/register', component: { template: '<div />' } },
  { path: '/admin/users', component: { template: '<div />' } },
]

function makeUser(): User {
  return {
    public_id: '00000000-0000-0000-0000-000000000001',
    display_name: 'mock-user',
    storage_namespace: 'USM001',
    avatar: '',
    role: 'user',
    status: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
  }
}

async function renderHost(
  path = '/user/boards',
  authenticated = true,
  configureRouter?: (router: Router) => void,
) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes,
  })
  configureRouter?.(router)
  await router.push(path)
  await router.isReady()
  const store = useUserStore()
  if (authenticated) store.setAuth(makeUser())
  let overlay: AccountOverlay | undefined
  const HostHarness = defineComponent({
    setup() {
      const currentRoute = useRoute()
      const currentRouter = useRouter()
      overlay = createAccountOverlay({
        currentUser: computed(() => store.user),
        reportError: vi.fn(),
        navigation: {
          replace: (location) => currentRouter.replace(location),
          resolve: (location) => currentRouter.resolve(location),
        },
        route: currentRoute,
      })
      provideAccountOverlay(overlay)
      return () => h(AccountDialogHost)
    },
  })

  const view = render(HostHarness, {
    global: {
      plugins: [router],
      stubs: {
        AccountDialog: {
          props: {
            visible: { type: Boolean, required: true },
            initialTab: { type: String, default: 'profile' },
            paymentReturn: { type: Object, default: null },
          },
          emits: ['update:visible'],
          template: `
            <section
              v-if="visible"
              role="dialog"
              aria-label="Account"
              :data-initial-tab="initialTab"
              :data-payment-token="paymentReturn?.publicToken"
              :data-payment-source="paymentReturn?.source"
            >
              <button type="button" aria-label="关闭账户管理" @click="$emit('update:visible', false)">
                关闭
              </button>
            </section>
          `,
        },
      },
    },
  })

  if (!overlay) throw new Error('Account Management overlay was not installed')
  return { overlay, router, store, ...view }
}

describe('AccountDialogHost', () => {
  beforeEach(() => {
    useUserStore().clearAuth()
    localStorage.clear()
  })

  it('shows the global dialog on authenticated user surfaces', async () => {
    const { overlay } = await renderHost()
    overlay.commands.open()

    expect(await screen.findByRole('dialog', { name: 'Account' })).toBeVisible()
  })

  it('forwards the controller initial tab to the global dialog', async () => {
    const { overlay } = await renderHost()
    overlay.commands.open('billing')

    expect(await screen.findByRole('dialog', { name: 'Account' })).toHaveAttribute(
      'data-initial-tab',
      'billing',
    )
  })

  it('opens the recharge pane from a valid Checkout return and cleans only payment query keys', async () => {
    const token = '0198cd29-f457-7e4b-b95e-6cfde41b7222'
    const { router } = await renderHost(
      `/user/boards?boardId=board-8&account=recharge&payment_return=${token}&checkout_return=success`,
    )

    const dialog = await screen.findByRole('dialog', { name: 'Account' })
    expect(dialog).toHaveAttribute('data-initial-tab', 'recharge')
    expect(dialog).toHaveAttribute('data-payment-token', token)
    expect(dialog).toHaveAttribute('data-payment-source', 'success')
    await waitFor(() => {
      expect(router.currentRoute.value.query).toEqual({ boardId: 'board-8' })
    })
    expect(dialog).toBeVisible()
  })

  it('retains a Checkout return until the current user session is restored', async () => {
    const token = '0198cd29-f457-7e4b-b95e-6cfde41b7222'
    const { router, store } = await renderHost(
      `/user/boards?account=recharge&payment_return=${token}&checkout_return=success`,
      false,
    )

    expect(screen.queryByRole('dialog', { name: 'Account' })).not.toBeInTheDocument()
    store.setAuth(makeUser())

    const dialog = await screen.findByRole('dialog', { name: 'Account' })
    expect(dialog).toHaveAttribute('data-payment-token', token)
    await waitFor(() => expect(router.currentRoute.value.query).toEqual({}))
  })

  it('ignores malformed Checkout return parameters', async () => {
    await renderHost('/user/boards?account=recharge&payment_return=short&checkout_return=success')

    expect(screen.queryByRole('dialog', { name: 'Account' })).not.toBeInTheDocument()
  })

  it('closes instead of consuming an unrelated navigation while Checkout query cleanup is pending', async () => {
    const token = '0198cd29-f457-7e4b-b95e-6cfde41b7222'
    const { router } = await renderHost(
      `/user/boards?account=recharge&payment_return=${token}&checkout_return=success`,
      true,
      (testRouter) => {
        testRouter.beforeEach((to, from) => {
          if (from.query.payment_return && !to.query.payment_return && to.path !== '/user/canvas') {
            return '/user/canvas?source=payment-cleanup'
          }
        })
      },
    )

    await waitFor(() =>
      expect(router.currentRoute.value.fullPath).toBe('/user/canvas?source=payment-cleanup'),
    )
    expect(screen.queryByRole('dialog', { name: 'Account' })).not.toBeInTheDocument()
  })

  it.each([['/admin/users'], ['/user/login'], ['/user/register']])(
    'does not host the dialog on excluded route %s',
    async (path) => {
      const { overlay } = await renderHost(path)
      overlay.commands.open()

      expect(screen.queryByRole('dialog', { name: 'Account' })).not.toBeInTheDocument()
    },
  )

  it('does not host the dialog without a current user', async () => {
    const { overlay, store } = await renderHost()
    store.clearAuth()
    overlay.commands.open()

    await waitFor(() => {
      expect(screen.queryByRole('dialog', { name: 'Account' })).not.toBeInTheDocument()
    })
  })

  it('closes the dialog on route changes', async () => {
    const { overlay, router } = await renderHost()
    overlay.commands.open()
    expect(await screen.findByRole('dialog', { name: 'Account' })).toBeVisible()

    await router.push('/user/canvas?boardId=demo')

    await waitFor(() => {
      expect(screen.queryByRole('dialog', { name: 'Account' })).not.toBeInTheDocument()
    })
  })

  it('closes the dialog when the current user becomes unavailable', async () => {
    const { overlay, store } = await renderHost()
    overlay.commands.open()
    expect(await screen.findByRole('dialog', { name: 'Account' })).toBeVisible()

    store.clearAuth()

    await waitFor(() => {
      expect(screen.queryByRole('dialog', { name: 'Account' })).not.toBeInTheDocument()
    })
  })
})
