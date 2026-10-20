import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'
import { i18n } from '../../../../core/i18n'
import { render, renderWithUser, screen, waitFor, within } from '../../../../test/render'
import { expectDomOrder } from '../../../../test/domOrder'
import { mockAppToast, type AppToastMocks } from '../../../../test/toast'
import UserAccountMenu from './UserAccountMenu.vue'
import type {
  UserAccountMenuItem,
  UserAccountMenuWorkspace,
  UserIdeaBalanceControl,
} from './userAccountMenu.types'

// Reka modal primitives lock body pointer events, which blocks user-event
// clicks inside the dialog. The AccountDialog harness precedent applies:
// stub the Dialog shell so these specs exercise the settings content.
const DialogRootStub = defineComponent({
  props: {
    open: { type: Boolean, required: true },
  },
  emits: ['update:open'],
  template: `
    <div v-if="open" @keydown.escape="$emit('update:open', false)">
      <slot />
    </div>
  `,
})

const DialogPassthroughStub = defineComponent({
  template: '<slot />',
})

const DialogContentStub = defineComponent({
  inheritAttrs: false,
  template: `
    <section role="dialog" tabindex="-1" v-bind="$attrs">
      <slot />
    </section>
  `,
})

// Member sections ride TanStack Query per workspace row. These menu-level
// specs cover the workspace row/rename/create flows, not members: stub the
// section controller (read-only empty roster) so no query client or
// transport is needed here. Member behavior lives in
// `WorkspaceMemberSection.component.spec.ts` with the real controller.
vi.mock('../../../../core/composables/useAppToast')
vi.mock('./useWorkspaceMemberSection', () => ({
  memberDisplayName: (entry: { userId: string }) => entry.userId,
  useWorkspaceMemberSection: () => ({
    roster: ref([]),
    myRole: ref(null),
    canManage: ref(false),
    isOwner: ref(false),
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

const DialogTitleStub = defineComponent({
  template: '<h2><slot /></h2>',
})

const dialogStubs = {
  DialogRoot: DialogRootStub,
  DialogPortal: DialogPassthroughStub,
  DialogOverlay: DialogPassthroughStub,
  DialogContent: DialogContentStub,
  DialogDescription: DialogPassthroughStub,
  DialogTitle: DialogTitleStub,
}

function workspaceControls(
  overrides?: Partial<UserAccountMenuWorkspace>,
): UserAccountMenuWorkspace {
  return {
    items: [
      {
        publicId: 'ws_personal1',
        name: 'Personal',
        organizationId: null,
        isDefault: true,
      },
      {
        publicId: 'ws_team2',
        name: 'Team',
        organizationId: 'org_c3',
        isDefault: false,
      },
    ],
    selectedId: 'ws_personal1',
    status: 'ready',
    onSelect: vi.fn(),
    onCreate: vi.fn(),
    onRename: vi.fn(),
    onDelete: vi.fn(),
    onRetry: vi.fn(),
    ...overrides,
  }
}

const profileItems: UserAccountMenuItem[] = [{ key: 'profile', label: 'Account', command: vi.fn() }]

async function openSubmenu(trigger: HTMLElement, firstOptionName: string | RegExp) {
  trigger.dispatchEvent(
    new PointerEvent('pointermove', {
      bubbles: true,
      cancelable: true,
      pointerType: 'mouse',
    }),
  )
  return screen.findByRole('menuitem', { name: firstOptionName })
}

describe('UserAccountMenu', () => {
  let toast: AppToastMocks
  beforeEach(() => {
    i18n.global.locale.value = 'en'
    toast = mockAppToast()
  })
  it('shows the account email above a separator in the user menu', async () => {
    const { user } = renderWithUser(UserAccountMenu, {
      props: {
        userEmail: 'alice@example.test',
        items: [{ key: 'profile', label: '账户管理', command: vi.fn() }],
      },
    })

    await user.click(screen.getByRole('button', { name: 'Account menu' }))

    const email = await screen.findByText('alice@example.test')
    const [separator] = screen.getAllByRole('separator')
    expect(email).toBeVisible()
    expect(separator).toBeVisible()
    expectDomOrder([email, separator])
  })

  it('renders the typed balance control before the avatar', async () => {
    const openCredits = vi.fn()
    const items: UserAccountMenuItem[] = [
      { key: 'profile', label: '账户管理', command: vi.fn() },
      { key: 'logout', label: '退出登录', command: vi.fn() },
    ]
    const balance: UserIdeaBalanceControl = {
      status: 'ready',
      availableIdeas: 1280,
      displayValue: '1,280',
      ariaLabel: 'Idea余额 1,280 Ideas',
      command: openCredits,
    }
    const { user } = renderWithUser(UserAccountMenu, { props: { items, balance } })

    const credits = screen.getByRole('button', { name: 'Idea余额 1,280 Ideas' })
    const accountMenu = screen.getByRole('button', { name: 'Account menu' })

    expect(credits).toBeVisible()
    expect(credits).toHaveTextContent('1,280')
    expect(credits).not.toHaveTextContent('Ideas')
    expect(credits).toHaveAttribute('title', 'Idea余额 1,280 Ideas')
    expect(credits.querySelector('svg')).not.toBeNull()
    expect(credits.nextElementSibling).toBe(accountMenu)

    await user.click(accountMenu)
    expect(await screen.findByText('账户管理')).toBeVisible()
    expect(screen.queryByText('Idea')).toBeNull()

    await user.click(credits)
    expect(openCredits).toHaveBeenCalledOnce()
  })

  it('renders an inert circular idea icon when anonymous', () => {
    const balance: UserIdeaBalanceControl = {
      status: 'anonymous',
      ariaLabel: 'Idea 标识',
    }
    render(UserAccountMenu, { props: { items: [], balance } })

    const indicator = screen.getByTitle('Idea 标识')
    expect(indicator).toBeVisible()
    expect(indicator.tagName.toLowerCase()).toBe('div')
    expect(screen.queryByText('加载中')).toBeNull()
  })

  it('renders guest pill action buttons when login and register items are provided', async () => {
    const openLogin = vi.fn()
    const openRegister = vi.fn()
    const items: UserAccountMenuItem[] = [
      { key: 'login', label: '登录', command: openLogin },
      { key: 'register', label: '免费注册', command: openRegister },
    ]
    const { user } = renderWithUser(UserAccountMenu, { props: { items } })

    const loginBtn = screen.getByRole('button', { name: 'Sign in' })
    const registerBtn = screen.getByRole('button', { name: 'Sign up for free' })

    expect(loginBtn).toBeVisible()
    expect(registerBtn).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Account menu' })).toBeNull()

    await user.click(loginBtn)
    expect(openLogin).toHaveBeenCalledOnce()

    await user.click(registerBtn)
    expect(openRegister).toHaveBeenCalledOnce()
  })

  it('keeps the balance control stable while credits are loading', () => {
    const items: UserAccountMenuItem[] = [{ key: 'profile', label: '账户管理', command: vi.fn() }]
    const balance: UserIdeaBalanceControl = {
      status: 'loading',
      displayValue: '加载中',
      ariaLabel: 'Idea余额 加载中',
      command: vi.fn(),
    }

    render(UserAccountMenu, { props: { items, balance } })

    const credits = screen.getByRole('button', { name: 'Idea余额 加载中' })
    expect(credits).toHaveAttribute('aria-busy', 'true')
    expect(credits).toHaveTextContent('加载中')
  })

  it('keeps a failed balance actionable without reporting it as busy', async () => {
    const openBilling = vi.fn()
    const balance: UserIdeaBalanceControl = {
      status: 'error',
      displayValue: '--',
      ariaLabel: 'Idea余额加载失败，点击查看账户',
      errorMessage: 'wallet unavailable',
      command: openBilling,
    }
    const { user } = renderWithUser(UserAccountMenu, { props: { items: [], balance } })

    const credits = screen.getByRole('button', { name: 'Idea余额加载失败，点击查看账户' })
    expect(credits).toHaveAttribute('aria-busy', 'false')
    expect(credits).toHaveAttribute('data-balance-status', 'error')
    expect(credits).toHaveTextContent('--')

    await user.click(credits)
    expect(openBilling).toHaveBeenCalledOnce()
  })

  it('keeps destructive menu intent semantic and actionable', async () => {
    const logout = vi.fn()
    const items: UserAccountMenuItem[] = [
      { key: 'logout', label: '退出登录', tone: 'danger', command: logout },
    ]
    const { user } = renderWithUser(UserAccountMenu, { props: { items } })

    await user.click(screen.getByRole('button', { name: 'Account menu' }))
    const logoutItem = await screen.findByRole('menuitem', { name: '退出登录' })

    expect(logoutItem).toHaveAttribute('data-control-tone', 'danger')
    await user.click(logoutItem)
    await waitFor(() => expect(logout).toHaveBeenCalledOnce())
  })

  it('restores focus and clears open state after Escape before reopening', async () => {
    const items: UserAccountMenuItem[] = [{ key: 'profile', label: '账户管理', command: vi.fn() }]
    const { user } = renderWithUser(UserAccountMenu, { props: { items } })
    const trigger = screen.getByRole('button', { name: 'Account menu' })

    await user.click(trigger)
    expect(await screen.findByRole('menuitem', { name: '账户管理' })).toBeVisible()
    expect(trigger).toHaveAttribute('aria-expanded', 'true')

    await user.keyboard('{Escape}')
    await waitFor(() => expect(trigger).toHaveAttribute('aria-expanded', 'false'))
    expect(trigger).toHaveFocus()

    await user.click(trigger)
    expect(await screen.findByRole('menuitem', { name: '账户管理' })).toBeVisible()
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
  })

  it('cleans up outside-click dismissal through the overlay lifecycle', async () => {
    const items: UserAccountMenuItem[] = [{ key: 'profile', label: '账户管理', command: vi.fn() }]
    const { user } = renderWithUser(UserAccountMenu, {
      props: { items },
    })
    const trigger = screen.getByRole('button', { name: 'Account menu' })

    await user.click(trigger)
    expect(await screen.findByRole('menuitem', { name: '账户管理' })).toBeVisible()
    await user.click(document.body)

    await waitFor(() => expect(trigger).toHaveAttribute('aria-expanded', 'false'))
    expect(trigger).toHaveFocus()
  })

  it('renders custom injected actions slot between credit balance and avatar', () => {
    const items: UserAccountMenuItem[] = [{ key: 'profile', label: '账户管理', command: vi.fn() }]
    const balance: UserIdeaBalanceControl = {
      status: 'ready',
      availableIdeas: 100,
      displayValue: '100',
      ariaLabel: 'Idea余额 100 Ideas',
      command: vi.fn(),
    }
    render(UserAccountMenu, {
      props: { items, balance },
      slots: {
        actions: '<button type="button" aria-label="custom-action">Custom Action</button>',
      },
    })

    const customAction = screen.getByRole('button', { name: 'custom-action' })
    const credits = screen.getByRole('button', { name: 'Idea余额 100 Ideas' })
    const accountMenu = screen.getByRole('button', { name: 'Account menu' })

    expect(customAction).toBeVisible()
    expect(credits.nextElementSibling).toBe(customAction)
    expect(customAction.nextElementSibling).toBe(accountMenu)
  })

  it('renders appearance submenu allowing tri-state theme switching', async () => {
    const items: UserAccountMenuItem[] = [{ key: 'profile', label: '账户管理', command: vi.fn() }]
    const { user } = renderWithUser(UserAccountMenu, { props: { items } })

    const trigger = screen.getByRole('button', { name: 'Account menu' })
    await user.click(trigger)

    const appearanceTrigger = await screen.findByRole('menuitem', { name: /Appearance/i })
    expect(appearanceTrigger).toBeVisible()
    const darkOption = await openSubmenu(appearanceTrigger, /Dark/i)
    expect(darkOption).toBeVisible()
    expect(screen.getByRole('menuitem', { name: /Light/i })).toBeVisible()
    expect(screen.getByRole('menuitem', { name: /System/i })).toBeVisible()

    await user.click(darkOption)
    expect(darkOption).toHaveAttribute('data-control-state', 'active')
  })

  it('renders language submenu allowing locale switching', async () => {
    const items: UserAccountMenuItem[] = [{ key: 'profile', label: '账户管理', command: vi.fn() }]
    const { user } = renderWithUser(UserAccountMenu, { props: { items } })

    const trigger = screen.getByRole('button', { name: 'Account menu' })
    await user.click(trigger)

    const languageTrigger = await screen.findByRole('menuitem', { name: /Language/i })
    expect(languageTrigger).toBeVisible()
    const zhOption = await openSubmenu(languageTrigger, '简体中文')
    expect(zhOption).toBeVisible()
    expect(screen.getByRole('menuitem', { name: 'English' })).toBeVisible()
    expect(screen.getByRole('menuitem', { name: 'Español' })).toBeVisible()
    expect(screen.getByRole('menuitem', { name: '한국어' })).toBeVisible()

    await user.click(zhOption)
    // The switch loads the zh-CN bundle asynchronously: await its completion
    // and switch back before the test ends, otherwise the in-flight promise
    // resolves during a later test (after its sync locale reset) and flips
    // the menu copy mid-test under parallel load. Selecting a locale closes
    // the menu, so reopen it first (labels below are the zh-CN copies).
    await waitFor(() => expect(i18n.global.locale.value).toBe('zh-CN'))
    await user.click(screen.getByRole('button', { name: '账户菜单' }))
    const languageTriggerZh = await screen.findByRole('menuitem', { name: '语言' })
    languageTriggerZh.dispatchEvent(
      new PointerEvent('pointermove', {
        bubbles: true,
        cancelable: true,
        pointerType: 'mouse',
      }),
    )
    await user.click(await screen.findByRole('menuitem', { name: 'English' }))
    await waitFor(() => expect(i18n.global.locale.value).toBe('en'))
  })

  it('places configured support channels after language and links the QR code safely', async () => {
    const supportChannels: import('../../../../core/composables/useSupportChannels').SupportChannel[] =
      [
        {
          id: 'discord',
          name: 'Discord',
          kind: 'discord',
          url: 'https://discord.gg/example',
          enabled: true,
          sort_order: 0,
        },
      ]
    const items: UserAccountMenuItem[] = [
      { key: 'profile', label: 'Account', command: vi.fn() },
      { key: 'logout', label: 'Sign out', tone: 'danger', command: vi.fn() },
    ]
    const { user } = renderWithUser(UserAccountMenu, {
      props: { items, supportChannels },
    })

    await user.click(screen.getByRole('button', { name: 'Account menu' }))

    const language = await screen.findByRole('menuitem', { name: /Language/i })
    const support = await screen.findByRole('menuitem', { name: /^(Support|支持)$/ })
    const signOutSeparator = screen.getAllByRole('separator').at(-1)

    const channel = await openSubmenu(support, 'Discord')
    await user.click(channel)
    const link = await screen.findByRole('link', { name: /Discord/ })
    expect(link).toHaveAttribute('href', 'https://discord.gg/example')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    expect(link.querySelector('svg')).not.toBeNull()
    expectDomOrder([language, support, signOutSeparator])
  })

  it('hides Support when no channels are available', async () => {
    const { user } = renderWithUser(UserAccountMenu, { props: { items: [] } })
    await user.click(screen.getByRole('button', { name: 'Account menu' }))
    expect(screen.queryByRole('menuitem', { name: 'Support' })).not.toBeInTheDocument()
  })

  it('renders user email at top with a separator and omits separator above appearance', async () => {
    const items: UserAccountMenuItem[] = [
      { key: 'profile', label: '账户管理', command: vi.fn() },
      { key: 'logout', label: '退出登录', tone: 'danger', command: vi.fn() },
    ]
    const { user } = renderWithUser(UserAccountMenu, {
      props: {
        items,
        userEmail: 'user@ideaboard.local',
      },
    })

    const trigger = screen.getByRole('button', { name: 'Account menu' })
    await user.click(trigger)

    const emailLabel = await screen.findByText('user@ideaboard.local')
    expect(emailLabel).toBeVisible()

    const separators = screen.getAllByRole('separator')
    // One separator after email, one after the workspace block, one before
    // danger item (logout)
    expect(separators).toHaveLength(3)

    // The first separator should immediately follow the email label container
    const emailContainer = emailLabel.closest('div') ?? emailLabel
    expect(emailContainer.nextElementSibling).toBe(separators[0])

    // There should be no separator directly between profile and appearance
    const profileItem = screen.getByRole('menuitem', { name: '账户管理' })
    const appearanceTrigger = screen.getByRole('menuitem', { name: /Appearance/i })
    expect(profileItem.nextElementSibling).toBe(appearanceTrigger)
  })

  describe('workspace switcher row', () => {
    async function openMenuWithWorkspace(workspace: UserAccountMenuWorkspace) {
      const view = renderWithUser(UserAccountMenu, {
        props: { items: profileItems, workspace },
      })
      await view.user.click(screen.getByRole('button', { name: 'Account menu' }))
      return view
    }

    async function openSettingsDialog(workspace: UserAccountMenuWorkspace) {
      const view = renderWithUser(UserAccountMenu, {
        props: { items: profileItems, workspace },
        global: { stubs: dialogStubs },
      })
      await view.user.click(screen.getByRole('button', { name: 'Account menu' }))
      await view.user.click(screen.getByRole('button', { name: 'Manage workspaces' }))
      await screen.findByRole('heading', { name: 'Workspaces' })
      return view
    }

    async function openWorkspaceSubmenu(trigger: HTMLElement) {
      trigger.dispatchEvent(
        new PointerEvent('pointermove', {
          bubbles: true,
          cancelable: true,
          pointerType: 'mouse',
        }),
      )
      return screen.findByRole('menuitem', { name: 'Team' })
    }

    it('places the workspace row after email and before account items', async () => {
      await openMenuWithWorkspace(workspaceControls())

      const workspaceTrigger = await screen.findByRole('menuitem', { name: 'Default' })
      const profileItem = await screen.findByRole('menuitem', { name: 'Account' })
      const appearanceTrigger = await screen.findByRole('menuitem', { name: /Appearance/i })
      expectDomOrder([workspaceTrigger, profileItem, appearanceTrigger])
    })

    it('opens workspaces on hover with the current check and no extra badge', async () => {
      await openMenuWithWorkspace(workspaceControls())

      const trigger = await screen.findByRole('menuitem', { name: 'Default' })
      const teamOption = await openWorkspaceSubmenu(trigger)

      expect(teamOption).toBeVisible()
      // Personal reads as Default in both the trigger row and the submenu
      // option: same shape as theme/language, no badge anywhere.
      const defaults = await screen.findAllByRole('menuitem', { name: 'Default' })
      expect(defaults).toHaveLength(2)
      const option = defaults.find((entry) => entry.getAttribute('aria-current') === 'true')
      expect(option).toBeDefined()
      expect(screen.queryByRole('menuitem', { name: 'Personal' })).not.toBeInTheDocument()
    })

    it('selects a workspace from the submenu and closes the menu', async () => {
      const workspace = workspaceControls()
      const view = await openMenuWithWorkspace(workspace)

      const trigger = await screen.findByRole('menuitem', { name: 'Default' })
      const teamOption = await openWorkspaceSubmenu(trigger)
      await view.user.click(teamOption)

      await waitFor(() => expect(workspace.onSelect).toHaveBeenCalledWith('ws_team2'))
      await waitFor(() =>
        expect(screen.getByRole('button', { name: 'Account menu' })).toHaveAttribute(
          'aria-expanded',
          'false',
        ),
      )
    })

    it('keeps retrying the list inside the menu without closing it', async () => {
      const workspace = workspaceControls({ status: 'error', errorMessage: 'List unavailable' })
      const view = await openMenuWithWorkspace(workspace)

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
      expect(screen.getByRole('button', { name: 'Account menu' })).toHaveAttribute(
        'aria-expanded',
        'true',
      )
    })

    it('opens settings on gear click without opening the submenu', async () => {
      const view = await openMenuWithWorkspace(workspaceControls())

      await view.user.click(screen.getByRole('button', { name: 'Manage workspaces' }))

      expect(await screen.findByRole('heading', { name: 'Workspaces' })).toBeVisible()
      expect(screen.queryByRole('menuitem', { name: 'Team' })).not.toBeInTheDocument()
    })

    it('opens the naming dialog with a unique default name and creates on confirm', async () => {
      const created = {
        publicId: 'ws_new4',
        name: 'New workspace',
        organizationId: null,
        isDefault: false,
      }
      const workspace = workspaceControls({ onCreate: vi.fn().mockResolvedValue(created) })
      const view = await openSettingsDialog(workspace)

      const addButton = screen.getByRole('button', { name: 'Add New' })
      expect(addButton).toBeEnabled()

      await view.user.click(addButton)

      const dialog = await screen.findByRole('dialog', { name: 'New workspace' })
      expect(dialog).toBeVisible()
      const nameInput = screen.getByLabelText('Workspace name')
      expect(nameInput).toHaveValue('New workspace')
      await waitFor(() => expect(nameInput).toHaveFocus())

      await view.user.click(screen.getByRole('button', { name: 'Create workspace' }))

      await waitFor(() => expect(workspace.onCreate).toHaveBeenCalledWith('New workspace'))
      await waitFor(() =>
        expect(screen.queryByRole('dialog', { name: 'New workspace' })).not.toBeInTheDocument(),
      )
      expect(screen.getByRole('heading', { name: 'Workspaces' })).toBeVisible()
    })

    it('creates with a typed name and focuses the new row with Members', async () => {
      const created = {
        publicId: 'ws_new4',
        name: 'Typed space',
        organizationId: null,
        isDefault: false,
      }
      const workspace = workspaceControls({ onCreate: vi.fn().mockResolvedValue(created) })
      const view = await openSettingsDialog(workspace)

      await view.user.click(screen.getByRole('button', { name: 'Add New' }))
      const nameInput = await screen.findByLabelText('Workspace name')
      await view.user.clear(nameInput)
      await view.user.type(nameInput, 'Typed space')
      await view.user.click(screen.getByRole('button', { name: 'Create workspace' }))

      await waitFor(() => expect(workspace.onCreate).toHaveBeenCalledWith('Typed space'))
      await view.rerender({
        items: profileItems,
        workspace: { ...workspace, items: [...workspace.items, created] },
      })

      const newRow = await screen.findByRole('button', { name: 'Typed space' })
      await waitFor(() => expect(newRow).toHaveAttribute('aria-current', 'page'))
      expect(await screen.findByRole('heading', { name: 'Typed space' })).toBeVisible()
      expect(screen.getByRole('heading', { name: 'Members' })).toBeVisible()
      await waitFor(() => expect(newRow).toHaveFocus())
      expect(screen.getByRole('heading', { name: 'Workspaces' })).toBeVisible()
    })

    it('toasts on an empty name without creating', async () => {
      const workspace = workspaceControls({ onCreate: vi.fn() })
      const view = await openSettingsDialog(workspace)

      await view.user.click(screen.getByRole('button', { name: 'Add New' }))
      const nameInput = await screen.findByLabelText('Workspace name')
      await view.user.clear(nameInput)
      await view.user.click(screen.getByRole('button', { name: 'Create workspace' }))

      await waitFor(() =>
        expect(toast.error).toHaveBeenCalledWith('Workspace name cannot be empty.'),
      )
      expect(workspace.onCreate).not.toHaveBeenCalled()
      expect(screen.getByRole('dialog', { name: 'New workspace' })).toBeVisible()
      expect(screen.getByRole('heading', { name: 'Workspaces' })).toBeVisible()
    })

    it('cancels the naming dialog without side effects', async () => {
      const workspace = workspaceControls({ onCreate: vi.fn() })
      const view = await openSettingsDialog(workspace)

      await view.user.click(screen.getByRole('button', { name: 'Add New' }))
      await screen.findByRole('dialog', { name: 'New workspace' })
      await view.user.click(screen.getByRole('button', { name: 'Cancel' }))

      expect(workspace.onCreate).not.toHaveBeenCalled()
      await waitFor(() =>
        expect(screen.queryByRole('dialog', { name: 'New workspace' })).not.toBeInTheDocument(),
      )
      expect(screen.getByRole('heading', { name: 'Workspaces' })).toBeVisible()
    })

    it('blocks a duplicate name inline without creating', async () => {
      const workspace = workspaceControls({ onCreate: vi.fn() })
      const view = await openSettingsDialog(workspace)

      await view.user.click(screen.getByRole('button', { name: 'Add New' }))
      const nameInput = await screen.findByLabelText('Workspace name')
      await view.user.clear(nameInput)
      await view.user.type(nameInput, 'Team')
      await view.user.click(screen.getByRole('button', { name: 'Create workspace' }))

      expect(await screen.findByText('A workspace with this name already exists.')).toBeVisible()
      expect(workspace.onCreate).not.toHaveBeenCalled()
      expect(screen.getByRole('dialog', { name: 'New workspace' })).toBeVisible()
    })

    it('generates an incremented default name when collision occurs', async () => {
      const items = [
        {
          publicId: 'ws_personal1',
          name: 'Personal',
          organizationId: null,
          isDefault: true,
        },
        {
          publicId: 'ws_item2',
          name: 'New workspace',
          organizationId: 'org_c2',
          isDefault: false,
        },
      ]
      const workspace = workspaceControls({
        items,
        onCreate: vi.fn().mockResolvedValue({
          publicId: 'ws_3',
          name: 'New workspace 2',
          organizationId: null,
          isDefault: false,
        }),
      })
      const view = await openSettingsDialog(workspace)

      await view.user.click(screen.getByRole('button', { name: 'Add New' }))
      const nameInput = await screen.findByLabelText('Workspace name')
      expect(nameInput).toHaveValue('New workspace 2')
      await view.user.click(screen.getByRole('button', { name: 'Create workspace' }))
      await waitFor(() => expect(workspace.onCreate).toHaveBeenCalledWith('New workspace 2'))
    })

    it('keeps the naming dialog open with an inline error when creation fails', async () => {
      const workspace = workspaceControls({
        onCreate: vi.fn().mockRejectedValue(new Error('boom')),
      })
      const view = await openSettingsDialog(workspace)

      await view.user.click(screen.getByRole('button', { name: 'Add New' }))
      await screen.findByLabelText('Workspace name')
      await view.user.click(screen.getByRole('button', { name: 'Create workspace' }))

      expect(
        await screen.findAllByText('Could not create the workspace. Please try again.'),
      ).not.toHaveLength(0)
      await waitFor(() =>
        expect(toast.error).toHaveBeenCalledWith(
          'Could not create the workspace. Please try again.',
        ),
      )
      expect(screen.getByRole('heading', { name: 'Workspaces' })).toBeVisible()
      expect(screen.getByRole('dialog', { name: 'New workspace' })).toBeVisible()
    })

    it('renames in the right panel and toasts on failure while keeping the old name', async () => {
      const workspace = workspaceControls({
        onRename: vi.fn().mockRejectedValue(new Error('boom')),
      })
      const view = await openSettingsDialog(workspace)

      await view.user.click(screen.getByRole('button', { name: 'Team' }))
      await view.user.click(screen.getByRole('button', { name: 'Rename Team' }))
      const renameInput = screen.getByLabelText('Rename Team')
      await view.user.clear(renameInput)
      await view.user.type(renameInput, 'Renamed')
      await view.user.keyboard('{Enter}')

      await waitFor(() => expect(workspace.onRename).toHaveBeenCalledWith('ws_team2', 'Renamed'))
      // Server-side rename failures go through toast; inline copy stays for
      // field-level validation (validateName) only.
      await waitFor(() =>
        expect(toast.error).toHaveBeenCalledWith(
          'Could not rename the workspace. The original name was kept.',
        ),
      )
      expect(
        screen.queryByText('Could not rename the workspace. The original name was kept.'),
      ).not.toBeInTheDocument()
      expect(screen.getByLabelText('Rename Team')).toHaveValue('Team')
    })

    it('cancels an inline rename on Escape without calling rename', async () => {
      const workspace = workspaceControls()
      const view = await openSettingsDialog(workspace)

      await view.user.click(screen.getByRole('button', { name: 'Team' }))
      await view.user.click(screen.getByRole('button', { name: 'Rename Team' }))
      const renameInput = screen.getByLabelText('Rename Team')
      await view.user.type(renameInput, ' X')
      await view.user.keyboard('{Escape}')

      expect(workspace.onRename).not.toHaveBeenCalled()
      expect(await screen.findByRole('button', { name: 'Rename Team' })).toBeVisible()
    })

    it('prevents renaming to an existing workspace name', async () => {
      const workspace = workspaceControls()
      const view = await openSettingsDialog(workspace)

      await view.user.click(screen.getByRole('button', { name: 'Team' }))
      await view.user.click(screen.getByRole('button', { name: 'Rename Team' }))
      const renameInput = screen.getByLabelText('Rename Team')
      await view.user.clear(renameInput)
      await view.user.type(renameInput, 'Default')
      await view.user.keyboard('{Enter}')

      expect(workspace.onRename).not.toHaveBeenCalled()
      expect(await screen.findByText('A workspace with this name already exists.')).toBeVisible()
    })

    it('hides the rename control for the Default workspace', async () => {
      const workspace = workspaceControls({ onRename: vi.fn() })
      await openSettingsDialog(workspace)

      expect(screen.queryByRole('button', { name: 'Rename Default' })).toBeNull()
      expect(workspace.onRename).not.toHaveBeenCalled()
    })

    it('renames a non-Default workspace', async () => {
      const renamed = {
        publicId: 'ws_team2',
        name: 'Renamed',
        organizationId: 'org_c3',
        isDefault: false,
      }
      const workspace = workspaceControls({
        onRename: vi.fn().mockResolvedValue(renamed),
      })
      const view = await openSettingsDialog(workspace)

      await view.user.click(screen.getByRole('button', { name: 'Team' }))
      await view.user.click(screen.getByRole('button', { name: 'Rename Team' }))
      const renameInput = screen.getByLabelText('Rename Team')
      await view.user.clear(renameInput)
      await view.user.type(renameInput, 'Renamed')
      await view.user.keyboard('{Enter}')

      await waitFor(() => expect(workspace.onRename).toHaveBeenCalledWith('ws_team2', 'Renamed'))
    })

    it('surfaces the server code when creation fails', async () => {
      const failure = Object.assign(new Error('request failed'), {
        code: 'workspace_default_conflict',
      })
      const workspace = workspaceControls({
        onCreate: vi.fn().mockRejectedValue(failure),
      })
      const view = await openSettingsDialog(workspace)

      await view.user.click(screen.getByRole('button', { name: 'Add New' }))
      await screen.findByLabelText('Workspace name')
      await view.user.click(screen.getByRole('button', { name: 'Create workspace' }))

      expect(
        await screen.findAllByText(
          'Could not create the workspace. Please try again. (workspace_default_conflict)',
        ),
      ).not.toHaveLength(0)
      await waitFor(() =>
        expect(toast.error).toHaveBeenCalledWith(
          'Could not create the workspace. Please try again. (workspace_default_conflict)',
        ),
      )
      expect(screen.getByRole('heading', { name: 'Workspaces' })).toBeVisible()
    })

    it('hides the delete section for the Default workspace', async () => {
      const workspace = workspaceControls()
      await openSettingsDialog(workspace)

      // Default is never deleted: the whole delete section stays hidden
      // (mirrors the rename pencil v-if), not a disabled button + note.
      expect(screen.queryByRole('button', { name: 'Delete' })).not.toBeInTheDocument()
      expect(screen.queryByText('Delete workspace')).not.toBeInTheDocument()
      expect(screen.queryByLabelText('Workspace deletion')).not.toBeInTheDocument()
      expect(workspace.onDelete).not.toHaveBeenCalled()
    })

    it('deletes a plain workspace through UserConfirmDialog with typed confirmation', async () => {
      const workspace = workspaceControls({
        onDelete: vi.fn().mockResolvedValue(undefined),
      })
      const view = await openSettingsDialog(workspace)

      await view.user.click(screen.getByRole('button', { name: 'Team' }))
      const deleteButton = screen.getByRole('button', { name: 'Delete' })
      expect(deleteButton).toBeEnabled()
      // Non-Default keeps the permanent-delete hint copy.
      expect(
        screen.getByText(
          'Permanently deletes the workspace immediately, including boards and files; billing records are kept. This cannot be undone.',
        ),
      ).toBeVisible()

      await view.user.click(deleteButton)
      const dialog = await screen.findByRole('dialog', { name: 'Delete workspace' })
      expect(dialog).toBeVisible()
      // UserConfirmDialog renders the message twice (sr-only description +
      // visible body), so assert on the set, not a single node.
      expect(
        within(dialog).getAllByText(
          'Delete "Team" permanently, including its boards and files. This cannot be undone.',
        ).length,
      ).toBeGreaterThan(0)

      // Typed confirmation: empty and mismatched names keep the destructive
      // control disabled.
      const confirmButton = within(dialog).getByRole('button', { name: 'Delete' })
      expect(confirmButton).toBeDisabled()
      const nameInput = within(dialog).getByLabelText('Type "Team" to confirm')
      await view.user.type(nameInput, 'Tea')
      expect(confirmButton).toBeDisabled()
      expect(workspace.onDelete).not.toHaveBeenCalled()

      await view.user.clear(nameInput)
      await view.user.type(nameInput, 'Team')
      expect(confirmButton).toBeEnabled()

      await view.user.click(confirmButton)
      await waitFor(() => expect(workspace.onDelete).toHaveBeenCalledWith('ws_team2'))
      await waitFor(() =>
        expect(screen.queryByRole('dialog', { name: 'Delete workspace' })).not.toBeInTheDocument(),
      )
      expect(screen.getByRole('heading', { name: 'Workspaces' })).toBeVisible()
    })

    it('cancels the delete confirm without side effects', async () => {
      const workspace = workspaceControls({ onDelete: vi.fn() })
      const view = await openSettingsDialog(workspace)

      await view.user.click(screen.getByRole('button', { name: 'Team' }))
      await view.user.click(screen.getByRole('button', { name: 'Delete' }))
      const dialog = await screen.findByRole('dialog', { name: 'Delete workspace' })
      expect(dialog).toBeVisible()

      await view.user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

      expect(workspace.onDelete).not.toHaveBeenCalled()
      await waitFor(() =>
        expect(screen.queryByRole('dialog', { name: 'Delete workspace' })).not.toBeInTheDocument(),
      )
      expect(screen.getByRole('heading', { name: 'Workspaces' })).toBeVisible()
    })

    it('locks the delete confirm while the commit is busy', async () => {
      let resolveDelete!: (value: unknown) => void
      const gate = new Promise((resolve) => {
        resolveDelete = resolve
      })
      const workspace = workspaceControls({ onDelete: vi.fn().mockReturnValue(gate) })
      const view = await openSettingsDialog(workspace)

      await view.user.click(screen.getByRole('button', { name: 'Team' }))
      await view.user.click(screen.getByRole('button', { name: 'Delete' }))
      const dialog = await screen.findByRole('dialog', { name: 'Delete workspace' })
      await view.user.type(within(dialog).getByLabelText('Type "Team" to confirm'), 'Team')
      await view.user.click(within(dialog).getByRole('button', { name: 'Delete' }))

      await waitFor(() => expect(workspace.onDelete).toHaveBeenCalledWith('ws_team2'))
      // Busy: UserConfirmDialog disables both controls and the parent
      // ignores dismissals, so the confirm stays open.
      expect(within(dialog).getByRole('button', { name: 'Delete' })).toBeDisabled()
      expect(within(dialog).getByRole('button', { name: 'Cancel' })).toBeDisabled()
      await view.user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
      expect(screen.getByRole('dialog', { name: 'Delete workspace' })).toBeVisible()
      expect(workspace.onDelete).toHaveBeenCalledTimes(1)

      resolveDelete(undefined)
      await waitFor(() =>
        expect(screen.queryByRole('dialog', { name: 'Delete workspace' })).not.toBeInTheDocument(),
      )
    })

    it('keeps settings open with an inline code when deletion fails', async () => {
      const failure = Object.assign(new Error('request failed'), {
        code: 'workspace_default_protected',
      })
      const workspace = workspaceControls({
        onDelete: vi.fn().mockRejectedValue(failure),
      })
      const view = await openSettingsDialog(workspace)

      await view.user.click(screen.getByRole('button', { name: 'Team' }))
      await view.user.click(screen.getByRole('button', { name: 'Delete' }))
      const dialog = await screen.findByRole('dialog', { name: 'Delete workspace' })
      await view.user.type(within(dialog).getByLabelText('Type "Team" to confirm'), 'Team')
      await view.user.click(within(dialog).getByRole('button', { name: 'Delete' }))

      expect(
        await screen.findByText(
          'Could not delete the workspace. Please try again. (workspace_default_protected)',
        ),
      ).toBeVisible()
      await waitFor(() =>
        expect(toast.error).toHaveBeenCalledWith(
          'Could not delete the workspace. Please try again. (workspace_default_protected)',
        ),
      )
      // Failure keeps the confirm open for retry or cancel.
      expect(screen.getByRole('dialog', { name: 'Delete workspace' })).toBeVisible()
      expect(screen.getByRole('heading', { name: 'Workspaces' })).toBeVisible()
    })
  })
})
