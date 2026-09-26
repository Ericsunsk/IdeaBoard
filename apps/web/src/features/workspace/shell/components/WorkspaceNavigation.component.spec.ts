import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { renderWithUser, waitFor } from '../../../../test/render'
import type { User } from '../../../../shared/api/sessionUser'
import { USER_BOARDS_PATH, USER_SEARCH_PATH } from '../../../../shared/constants/routePaths'
import { createAuthOverlay, provideAuthOverlay, type AuthOverlay } from '../../../auth'
import WorkspaceNavigation from './WorkspaceNavigation.vue'

const session = vi.hoisted(() => ({
  user: null as User | null,
  ensureSession: vi.fn(),
}))

vi.mock('../../../auth/session/userSession', () => ({
  useUserStore: () => ({
    get user() {
      return session.user
    },
    ensureSession: session.ensureSession,
  }),
}))

function makeUser(): User {
  return {
    public_id: '00000000-0000-0000-0000-000000000001',
    display_name: 'demo-user',
    storage_namespace: 'USR001',
    avatar: 'https://example.test/avatar.png',
    role: 'user',
    status: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }
}

async function renderNavigation(path = USER_SEARCH_PATH): Promise<{
  overlay: AuthOverlay
  router: ReturnType<typeof createRouter>
  view: ReturnType<typeof renderWithUser>
}> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: USER_SEARCH_PATH, component: { template: '<div>Search</div>' } },
      { path: USER_BOARDS_PATH, component: { template: '<div>Workspace</div>' } },
    ],
  })
  await router.push(path)
  await router.isReady()

  let overlay: AuthOverlay | null = null
  const Harness = defineComponent({
    setup() {
      overlay = createAuthOverlay()
      provideAuthOverlay(overlay)
      return () => h(WorkspaceNavigation)
    },
  })
  const view = renderWithUser(Harness, { global: { plugins: [router] } })
  if (!overlay) throw new Error('Auth overlay harness failed to initialize')

  return { overlay, router, view }
}

describe('WorkspaceNavigation', () => {
  beforeEach(() => {
    session.user = null
    session.ensureSession.mockReset()
  })

  it('keeps a stale cached user on the current page and opens the login overlay', async () => {
    session.user = makeUser()
    session.ensureSession.mockResolvedValue(null)
    const { overlay, router, view } = await renderNavigation()

    await view.user.click(view.getByRole('link', { name: 'Workspace' }))

    await waitFor(() => expect(overlay.viewModel.mode.value).toBe('login'))
    expect(session.ensureSession).toHaveBeenCalledWith({ fresh: true })
    expect(router.currentRoute.value.fullPath).toBe(USER_SEARCH_PATH)
    expect(overlay.commands.consumePostAuthPath()).toBe(USER_BOARDS_PATH)
  })

  it('fails closed on a session validation error without leaving the current page', async () => {
    session.user = makeUser()
    session.ensureSession.mockRejectedValue(new Error('session unavailable'))
    const { overlay, router, view } = await renderNavigation()

    await view.user.click(view.getByRole('link', { name: 'Workspace' }))

    await waitFor(() => expect(overlay.viewModel.mode.value).toBe('login'))
    expect(router.currentRoute.value.fullPath).toBe(USER_SEARCH_PATH)
    expect(overlay.commands.consumePostAuthPath()).toBe(USER_BOARDS_PATH)
  })

  it('navigates only after the session seam confirms the user', async () => {
    const user = makeUser()
    session.user = user
    session.ensureSession.mockResolvedValue(user)
    const { overlay, router, view } = await renderNavigation()

    await view.user.click(view.getByRole('link', { name: 'Workspace' }))

    await waitFor(() => expect(router.currentRoute.value.fullPath).toBe(USER_BOARDS_PATH))
    expect(session.ensureSession).toHaveBeenCalledWith({ fresh: true })
    expect(overlay.viewModel.visible.value).toBe(false)
  })

  it('opens login without leaving a public Workspace or storing redundant navigation', async () => {
    session.ensureSession.mockResolvedValue(null)
    const { overlay, router, view } = await renderNavigation(USER_BOARDS_PATH)

    await view.user.click(view.getByRole('link', { name: 'Workspace' }))

    await waitFor(() => expect(overlay.viewModel.mode.value).toBe('login'))
    expect(router.currentRoute.value.fullPath).toBe(USER_BOARDS_PATH)
    expect(overlay.commands.consumePostAuthPath()).toBeNull()
  })
})
