import { defineComponent, h, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser } from '../../../../test/render'
import type { User } from '../../../../api/sessionUserType'
import { USER_BOARDS_PATH, USER_SEARCH_PATH } from '../../routePaths'
import { createAuthOverlay, provideAuthOverlay } from '../../../auth'
import { createAccountOverlay, provideAccountOverlay } from '../../../account'
import WorkspaceHeader from './WorkspaceHeader.vue'

vi.mock('../../../../core/composables/useSupportChannels', () => ({
  useSupportChannels: () => ({ channels: [] }),
}))

const currentUser = ref<User | null>(null)

vi.mock('../../../auth/session/userSession', () => ({
  useUserStore: () => ({
    get user() {
      return currentUser.value
    },
    logout: vi.fn(),
  }),
}))

vi.mock('../../../account', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../../account')>()
  return {
    ...actual,
    useCreditWallet: () => ({
      state: ref({
        status: 'ready',
        availableCredits: 100,
      }),
    }),
  }
})

async function renderHeader(props: { sticky?: boolean } = {}) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: USER_SEARCH_PATH, component: { template: '<div>Search</div>' } },
      { path: USER_BOARDS_PATH, component: { template: '<div>Boards</div>' } },
    ],
  })
  await router.push(USER_SEARCH_PATH)
  await router.isReady()

  const Harness = defineComponent({
    setup() {
      const authOverlay = createAuthOverlay()
      provideAuthOverlay(authOverlay)
      const accountOverlay = createAccountOverlay({
        currentUser,
        reportError: vi.fn(),
        navigation: {
          replace: vi.fn(),
          resolve: (loc) => ({ fullPath: loc.path }),
        },
        route: { fullPath: USER_SEARCH_PATH, hash: '', path: USER_SEARCH_PATH, query: {} },
      })
      provideAccountOverlay(accountOverlay)
      return () => h(WorkspaceHeader, props)
    },
  })

  // The header hosts the workspace switcher, which reads the shared query
  // client at setup. Anonymous users keep the list query disabled, so no
  // network fires here.
  const queryClient = new QueryClient()
  return renderWithUser(Harness, {
    global: { plugins: [router, [VueQueryPlugin, { queryClient }]] },
  })
}

describe('WorkspaceHeader', () => {
  beforeEach(() => {
    currentUser.value = null
    useMswHandlers(
      http.get('*/api/v1/user/notifications', () =>
        HttpResponse.json({ data: { items: [], next_cursor: null } }),
      ),
      http.get('*/api/v1/user/notifications/unread-count', () =>
        HttpResponse.json({ data: { count: 0 } }),
      ),
      http.get('*/api/v1/workspaces', () => HttpResponse.json({ data: [] })),
      http.get('*/api/auth/get-session', () => HttpResponse.json(null)),
    )
  })

  it('renders brand, navigation and account controls by default', async () => {
    const view = await renderHeader()
    const header = view.container.querySelector('header')
    expect(header).toBeTruthy()

    const nav = view.getByRole('navigation')
    expect(nav).toBeTruthy()
  })

  it('renders in non-sticky layout when requested', async () => {
    const view = await renderHeader({ sticky: false })
    const header = view.container.querySelector('header')
    expect(header).toBeTruthy()
  })

  it('does not render notification bell when user is guest', async () => {
    currentUser.value = null
    const view = await renderHeader()
    expect(view.container.querySelector('[data-testid="notification-bell-btn"]')).toBeNull()
  })

  it('renders notification bell when user is authenticated', async () => {
    currentUser.value = {
      public_id: 'usr_auth_1',
      display_name: 'Member',
      avatar: '',
      role: 'user',
    }
    const view = await renderHeader()
    const bell = await view.findByTestId('notification-bell-btn')
    expect(bell).toBeInTheDocument()
  })
})
