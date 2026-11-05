// Guest workspace toast regression: unauthenticated search mount must not pop
// "Could not load workspaces", while a signed-in load failure still toasts
// once and keeps retry available.
import { beforeEach, describe, expect, test, vi } from 'vitest'
import { ref } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import type { RenderOptions } from '@testing-library/vue'
import { i18n } from '../../../../core/i18n'
import { renderWithUser as renderWithUserBase, screen, waitFor } from '../../../../test/render'
import { mockAppToast, type AppToastMocks } from '../../../../test/toast'
import UserAccountMenu from './UserAccountMenu.vue'
import { fetchBillingAccount } from '../../session/billing-account/resource'
import type { UserAccountMenuItem, UserAccountMenuWorkspace } from './userAccountMenu.types'

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

function renderWithUser<C>(view: C, options?: RenderOptions<C>) {
  return renderWithUserBase(view, withQuery(options))
}

vi.mock('../../../../core/composables/useAppToast')
vi.mock('../../session/billing-account/resource', async (load) => ({
  ...(await load<typeof import('../../session/billing-account/resource')>()),
  fetchBillingAccount: vi.fn().mockRejectedValue(new Error('no billing account')),
}))
vi.mock('../../session/workspace-settings', async (load) => ({
  ...(await load<typeof import('../../session/workspace-settings')>()),
  memberDisplayName: (entry: { userId: string }) => entry.userId,
  useWorkspaceMemberSection: () => ({
    roster: ref([]),
    myRole: ref(null),
    canManage: ref(false),
    isOwner: ref(false),
    isDefaultWorkspace: ref(false),
    isLoading: ref(false),
    hasLoadError: ref(false),
    retryRoster: vi.fn(),
    isPending: () => false,
    inviteEmail: ref(''),
    inviteRole: ref('member'),
    pickInviteRole: vi.fn(),
    inviteMember: vi.fn(),
    changeMemberRole: vi.fn(),
    confirmingRemoveId: ref(null),
    requestRemove: vi.fn(),
    cancelRemove: vi.fn(),
    confirmRemove: vi.fn(),
  }),
}))
vi.mock('../../session/quota/useMemberQuotaSection', () => ({
  useMemberQuotaSection: () => ({
    selfQuota: ref(null),
    isSelfLoading: ref(false),
    hasSelfError: ref(false),
    isSelfExhausted: ref(false),
    retrySelf: vi.fn(),
    quotas: ref([]),
    isListLoading: ref(false),
    hasListError: ref(false),
    retryList: vi.fn(),
    canManage: ref(false),
    memberOptions: ref([]),
    memberLabel: (userId: string) => userId,
    targetUserId: ref(''),
    targetPeriod: ref('daily'),
    limitDraft: ref(''),
    isSaving: ref(false),
    saveError: ref(null),
    pickTargetUser: vi.fn(),
    pickTargetPeriod: vi.fn(),
    saveQuota: vi.fn(),
    memberRole: () => null,
    canAdjustTarget: () => false,
  }),
}))

const guestItems: UserAccountMenuItem[] = [
  { key: 'login', label: '登录', command: vi.fn() },
  { key: 'register', label: '免费注册', command: vi.fn() },
]
const profileItems: UserAccountMenuItem[] = [{ key: 'profile', label: 'Account', command: vi.fn() }]

function workspaceControls(
  overrides?: Partial<UserAccountMenuWorkspace>,
): UserAccountMenuWorkspace {
  return {
    items: [],
    selectedId: null,
    status: 'error',
    errorMessage: 'Could not load workspaces.',
    onSelect: vi.fn(),
    onCreate: vi.fn(),
    onRename: vi.fn(),
    onDelete: vi.fn(),
    onRetry: vi.fn(),
    ...overrides,
  }
}

describe('UserAccountMenu guest workspace toast', () => {
  let toast: AppToastMocks

  beforeEach(() => {
    i18n.global.locale.value = 'en'
    toast = mockAppToast()
    vi.mocked(fetchBillingAccount).mockReset().mockResolvedValue({
      billingAccountId: 'ba_current',
      workspaceId: 'ws_personal1',
      isShared: false,
      canManageMemberQuotas: true,
      consumptionScope: 'account',
    })
  })

  test('guest mount with a workspace failure stays silent', async () => {
    renderWithUser(UserAccountMenu, {
      props: { items: guestItems, workspace: workspaceControls() },
    })

    expect(await screen.findByRole('button', { name: 'Sign in' })).toBeVisible()
    expect(toast.error).not.toHaveBeenCalled()
  })

  test('signed-in mount with a workspace failure still toasts and retries', async () => {
    const workspace = workspaceControls({
      items: [
        {
          publicId: 'ws_personal1',
          name: 'Personal',
          organizationId: null,
          isDefault: true,
        },
      ],
      selectedId: 'ws_personal1',
    })
    const view = renderWithUser(UserAccountMenu, {
      props: { items: profileItems, workspace },
    })

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Could not load workspaces.'))

    await view.user.click(screen.getByRole('button', { name: 'Account menu' }))
    const trigger = await screen.findByRole('menuitem', { name: 'Default' })
    trigger.dispatchEvent(
      new PointerEvent('pointermove', {
        bubbles: true,
        cancelable: true,
        pointerType: 'mouse',
      }),
    )
    const retryOption = await screen.findByRole('menuitem', { name: 'Retry' })
    await view.user.click(retryOption)

    expect(workspace.onRetry).toHaveBeenCalledOnce()
  })
})
