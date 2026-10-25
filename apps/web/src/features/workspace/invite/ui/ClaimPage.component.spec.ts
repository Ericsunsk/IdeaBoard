import type { MaybeRefOrGetter } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import type { RenderOptions } from '@testing-library/vue'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import { mockAppToast, type AppToastMocks } from '../../../../test/toast'
import ClaimPage from './ClaimPage.vue'
import type { WorkspaceInvitePreview } from '../../selection/workspaceInvitesResource'

const mocks = vi.hoisted(() => ({
  route: {
    path: '/invite/claim',
    fullPath: '/invite/claim?token=tok_test',
    query: {} as Record<string, string | undefined>,
  },
  routerPush: vi.fn(),
  userStore: {
    user: null as { id: string; email: string } | null,
  },
  previewData: null as WorkspaceInvitePreview | null,
  previewError: null as Error | null,
  acceptToken: vi.fn(),
  declineToken: vi.fn(),
}))

let toast: AppToastMocks

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ push: mocks.routerPush }),
}))

vi.mock('../../../../core/composables/useAppToast')

vi.mock('../../../auth', () => ({
  useUserStore: () => mocks.userStore,
}))

vi.mock('../../selection/workspaceInvitesResource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../selection/workspaceInvitesResource')>()
  return {
    ...actual,
    workspaceInviteByTokenQueryOptions: (token: MaybeRefOrGetter<string>) =>
      actual.workspaceInviteByTokenQueryOptions(token, () => {
        if (mocks.previewError) throw mocks.previewError
        return Promise.resolve(mocks.previewData!)
      }),
    acceptWorkspaceInviteByToken: (tok: string) => mocks.acceptToken(tok),
    declineWorkspaceInviteByToken: (tok: string) => mocks.declineToken(tok),
  }
})

function withQuery<C>(options?: RenderOptions<C>): RenderOptions<C> {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return {
    ...options,
    global: {
      ...options?.global,
      plugins: [...(options?.global?.plugins ?? []), [VueQueryPlugin, { queryClient }]],
    },
  }
}

function renderClaimPage() {
  return renderWithUser(ClaimPage, withQuery())
}

describe('ClaimPage', () => {
  beforeEach(() => {
    toast = mockAppToast()
    mocks.routerPush.mockClear()
    mocks.acceptToken.mockClear()
    mocks.declineToken.mockClear()
    mocks.previewData = null
    mocks.previewError = null
    mocks.userStore.user = null
    mocks.route.query = {}
    mocks.route.fullPath = '/invite/claim'
  })

  it('renders invalid token state when token query parameter is absent', async () => {
    mocks.route.query = {}
    const { user } = renderClaimPage()

    expect(await screen.findByText('Invalid invitation link')).toBeVisible()
    expect(
      screen.getByText(/The invitation link is missing or malformed/),
    ).toBeVisible()

    await user.click(screen.getByRole('button', { name: 'Back to home' }))
    expect(mocks.routerPush).toHaveBeenCalledWith('/')
  })

  it('renders not found state when invitation preview lookup fails', async () => {
    mocks.route.query = { token: 'tok_missing' }
    mocks.previewError = new Error('Not found')
    const { user } = renderClaimPage()

    expect(await screen.findByText('Invitation not found')).toBeVisible()
    expect(
      screen.getByText(/This invitation no longer exists or the link has already been used/),
    ).toBeVisible()

    await user.click(screen.getByRole('button', { name: 'Back to home' }))
    expect(mocks.routerPush).toHaveBeenCalledWith('/')
  })

  it('renders expired state when invitation status is expired', async () => {
    mocks.route.query = { token: 'tok_expired' }
    mocks.previewData = {
      workspace_id: 'ws_1',
      workspace_name: 'Acme Design',
      role: 'member',
      status: 'expired',
      expires_at: new Date(Date.now() - 3600000).toISOString(),
    }
    renderClaimPage()

    expect(await screen.findByText('Invitation expired')).toBeVisible()
    expect(
      screen.getByText(/This invitation link has expired/),
    ).toBeVisible()
  })

  it('renders already accepted state when invitation status is accepted', async () => {
    mocks.route.query = { token: 'tok_accepted' }
    mocks.previewData = {
      workspace_id: 'ws_1',
      workspace_name: 'Acme Design',
      role: 'member',
      status: 'accepted',
      expires_at: new Date(Date.now() + 86400000).toISOString(),
    }
    const { user } = renderClaimPage()

    expect(await screen.findByText('Already a member')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Go to workspace' }))
    expect(mocks.routerPush).toHaveBeenCalledWith('/user/boards')
  })

  it('handles unauthenticated state and redirects to sign in', async () => {
    mocks.route.query = { token: 'tok_valid' }
    mocks.route.fullPath = '/invite/claim?token=tok_valid'
    mocks.previewData = {
      workspace_id: 'ws_1',
      workspace_name: 'Acme Design',
      inviter_name: 'Alice',
      role: 'admin',
      status: 'pending',
      expires_at: new Date(Date.now() + 86400000 * 5).toISOString(),
    }
    mocks.userStore.user = null
    const { user } = renderClaimPage()

    expect(await screen.findByText("You're invited to join Acme Design")).toBeVisible()
    expect(screen.getByText('Alice invited you to collaborate as Admin.')).toBeVisible()

    const signInButton = screen.getByRole('button', { name: 'Sign in to accept' })
    expect(signInButton).toBeVisible()

    await user.click(signInButton)
    expect(mocks.routerPush).toHaveBeenCalledWith({
      path: '/user/login',
      query: { redirect: '/invite/claim?token=tok_valid' },
    })
  })

  it('handles authenticated state and accepts invitation', async () => {
    mocks.route.query = { token: 'tok_valid' }
    mocks.previewData = {
      workspace_id: 'ws_1',
      workspace_name: 'Acme Design',
      inviter_name: 'Alice',
      role: 'member',
      status: 'pending',
      expires_at: new Date(Date.now() + 86400000 * 5).toISOString(),
    }
    mocks.userStore.user = { id: 'usr_me', email: 'me@example.com' }
    mocks.acceptToken.mockResolvedValueOnce({
      status: 'accepted',
      workspace_id: 'ws_1',
      user_id: 'usr_me',
      role: 'member',
    })

    const { user } = renderClaimPage()

    expect(await screen.findByText("You're invited to join Acme Design")).toBeVisible()
    const acceptBtn = screen.getByRole('button', { name: 'Accept invitation' })
    const declineBtn = screen.getByRole('button', { name: 'Decline' })
    expect(acceptBtn).toBeVisible()
    expect(declineBtn).toBeVisible()

    await user.click(acceptBtn)
    await waitFor(() => expect(mocks.acceptToken).toHaveBeenCalledWith('tok_valid'))
    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith('You joined Acme Design.'),
    )
    expect(mocks.routerPush).toHaveBeenCalledWith('/user/boards')
  })

  it('handles authenticated state and declines invitation', async () => {
    mocks.route.query = { token: 'tok_valid' }
    mocks.previewData = {
      workspace_id: 'ws_1',
      workspace_name: 'Acme Design',
      role: 'member',
      status: 'pending',
      expires_at: new Date(Date.now() + 86400000 * 5).toISOString(),
    }
    mocks.userStore.user = { id: 'usr_me', email: 'me@example.com' }
    mocks.declineToken.mockResolvedValueOnce({
      status: 'declined',
      workspace_id: 'ws_1',
      user_id: 'usr_me',
      role: 'member',
    })

    const { user } = renderClaimPage()

    expect(await screen.findByText("You're invited to join Acme Design")).toBeVisible()
    const declineBtn = screen.getByRole('button', { name: 'Decline' })

    await user.click(declineBtn)
    await waitFor(() => expect(mocks.declineToken).toHaveBeenCalledWith('tok_valid'))
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith('Invitation declined.'))
    expect(await screen.findByText('Invitation unavailable')).toBeVisible()
  })
})
