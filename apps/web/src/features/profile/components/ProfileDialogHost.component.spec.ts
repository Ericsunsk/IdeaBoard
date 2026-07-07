import { createRouter, createMemoryHistory } from 'vue-router'
import type { Router, RouteRecordRaw } from 'vue-router'
import { beforeEach, describe, expect, it } from 'vitest'
import { render, screen, waitFor } from '../../../test/render'
import { useUserStore } from '../../../app/stores/user'
import type { User } from '../../../shared/api/userSession'
import { closeProfileDialog, openProfileDialog } from '../useProfileDialogController'
import ProfileDialogHost from './ProfileDialogHost.vue'

const routes: RouteRecordRaw[] = [
  { path: '/user/boards', component: { template: '<div />' } },
  { path: '/user/canvas', component: { template: '<div />' } },
  { path: '/user/login', component: { template: '<div />' } },
  { path: '/user/register', component: { template: '<div />' } },
  { path: '/admin/users', component: { template: '<div />' } },
]

function makeUser(): User {
  return {
    id: 1,
    uid: 'USM001',
    username: 'mock-user',
    auth_user_id: 'auth-user-route',
    avatar: '',
    role: 'user',
    status: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }
}

async function createTestRouter(path: string): Promise<Router> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes,
  })
  await router.push(path)
  await router.isReady()
  return router
}

async function renderHost(path = '/user/boards') {
  const router = await createTestRouter(path)
  const store = useUserStore()
  store.setAuth(makeUser())

  const view = render(ProfileDialogHost, {
    global: {
      plugins: [router],
      stubs: {
        ProfileDialog: {
          props: {
            visible: { type: Boolean, required: true },
          },
          emits: ['update:visible'],
          template: `
            <section v-if="visible" role="dialog" aria-label="账户管理">
              <button type="button" aria-label="关闭账户管理" @click="$emit('update:visible', false)">
                关闭
              </button>
            </section>
          `,
        },
      },
    },
  })

  return { router, store, ...view }
}

describe('ProfileDialogHost', () => {
  beforeEach(() => {
    closeProfileDialog()
    useUserStore().clearAuth()
    localStorage.clear()
  })

  it('shows the global dialog on authenticated user surfaces', async () => {
    openProfileDialog()

    await renderHost()

    expect(screen.getByRole('dialog', { name: '账户管理' })).toBeVisible()
  })

  it.each([['/admin/users'], ['/user/login'], ['/user/register']])(
    'does not host the dialog on excluded route %s',
    async (path) => {
      openProfileDialog()

      await renderHost(path)

      expect(screen.queryByRole('dialog', { name: '账户管理' })).not.toBeInTheDocument()
    },
  )

  it('does not host the dialog without a current user', async () => {
    const { store } = await renderHost()
    store.clearAuth()
    openProfileDialog()

    await waitFor(() => {
      expect(screen.queryByRole('dialog', { name: '账户管理' })).not.toBeInTheDocument()
    })
  })

  it('closes the dialog on route changes', async () => {
    openProfileDialog()
    const { router } = await renderHost()
    expect(screen.getByRole('dialog', { name: '账户管理' })).toBeVisible()

    await router.push('/user/canvas?boardId=demo')

    await waitFor(() => {
      expect(screen.queryByRole('dialog', { name: '账户管理' })).not.toBeInTheDocument()
    })
  })

  it('closes the dialog when the current user becomes unavailable', async () => {
    openProfileDialog()
    const { store } = await renderHost()
    expect(screen.getByRole('dialog', { name: '账户管理' })).toBeVisible()

    store.clearAuth()

    await waitFor(() => {
      expect(screen.queryByRole('dialog', { name: '账户管理' })).not.toBeInTheDocument()
    })
  })
})
