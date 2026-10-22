import { describe, expect, it, vi } from 'vitest'

vi.mock('../../../../../core/composables/useSupportChannels', () => ({
  useSupportChannels: () => ({ channels: [] }),
}))
vi.mock('../../../../account/ui/chrome/useWorkspaceMemberSection', () => ({
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
import { h, ref } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import type { RenderOptions } from '@testing-library/vue'
import {
  render,
  renderWithUser as renderWithUserBase,
  screen,
  waitFor,
} from '../../../../../test/render'
import CanvasChrome from './CanvasChrome.vue'

// The avatar menu mounts the real workspace settings dialog, whose billing
// context rides TanStack Query. Provide a client like the account menu
// harness does so these specs exercise the menu rows, not the provider.
function renderWithUser<C>(view: C, options?: RenderOptions<C>) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return renderWithUserBase(view, {
    ...options,
    global: {
      ...options?.global,
      plugins: [...(options?.global?.plugins ?? []), [VueQueryPlugin, { queryClient }]],
    },
  })
}

describe('CanvasChrome', () => {
  it('keeps only accessible viewport zoom controls without autosave feedback controls', () => {
    render(CanvasChrome, {
      props: {
        isInert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: null,
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          isReadOnly: false,
          catalogRecovery: { status: 'idle' },
          archived: { isArchived: false, isRestoring: false },
          setNameInputRef: vi.fn(),
        },
        account: {
          balance: {
            status: 'ready',
            availableIdeas: 0,
            displayValue: '0',
            ariaLabel: 'Idea余额 0 Ideas',
            command: vi.fn(),
          },
          menuItems: [],
        },
        tools: {
          activeTool: 'select',
          isLibraryMounted: false,
          isLibraryOpen: false,
          isMinimapEnabled: false,
        },
        status: { isExportDisabled: false, zoomLevel: 100 },
      },
    })

    expect(screen.queryByText('保存中')).toBeNull()
    expect(screen.queryByText('恢复保存中')).toBeNull()
    expect(screen.queryByText('远端未同步')).toBeNull()
    expect(screen.queryByText('保存冲突')).toBeNull()
    expect(screen.queryByText('未保存')).toBeNull()
    expect(screen.queryByRole('button', { name: '重试画布自动保存' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Zoom Out' })).toBeVisible()
    expect(screen.getByText('100%')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Zoom In' })).toBeVisible()
  })

  it('places the credits balance immediately before the share control', () => {
    render(CanvasChrome, {
      props: {
        isInert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: null,
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          isReadOnly: false,
          catalogRecovery: { status: 'idle' },
          archived: { isArchived: false, isRestoring: false },
          setNameInputRef: vi.fn(),
        },
        account: {
          balance: {
            status: 'ready',
            availableIdeas: 100,
            displayValue: '100',
            ariaLabel: 'Idea余额 100 Ideas',
            command: vi.fn(),
          },
          menuItems: [{ key: 'logout', label: '退出登录', command: vi.fn() }],
        },
        tools: {
          activeTool: 'select',
          isLibraryMounted: false,
          isLibraryOpen: false,
          isMinimapEnabled: false,
        },
        status: { isExportDisabled: false, zoomLevel: 100 },
      },
    })

    const credits = screen.getByRole('button', { name: 'Idea余额 100 Ideas' })
    const share = screen.getByRole('button', { name: 'Share' })

    expect(share.previousElementSibling).toBe(credits)
    expect(share).toBeEnabled()
    expect(share).toHaveAttribute('title', 'Share')
    expect(screen.getAllByRole('button', { name: 'Idea余额 100 Ideas' })).toHaveLength(1)
    expect(credits).toHaveTextContent('100')
    expect(credits).not.toHaveTextContent('Ideas')
  })

  it('emits the share intent when the share control is activated', async () => {
    const { emitted, user } = renderWithUser(CanvasChrome, {
      props: {
        isInert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: 'board-a',
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          isReadOnly: false,
          catalogRecovery: { status: 'idle' },
          archived: { isArchived: false, isRestoring: false },
          setNameInputRef: vi.fn(),
        },
        account: {
          balance: {
            status: 'ready',
            availableIdeas: 100,
            displayValue: '100',
            ariaLabel: 'Idea余额 100 Ideas',
            command: vi.fn(),
          },
          menuItems: [],
        },
        tools: {
          activeTool: 'select',
          isLibraryMounted: false,
          isLibraryOpen: false,
          isMinimapEnabled: false,
        },
        status: { isExportDisabled: false, zoomLevel: 100 },
      },
    })

    await user.click(screen.getByRole('button', { name: 'Share' }))

    expect(emitted()['share']).toHaveLength(1)
  })

  it('exposes an executable catalog reload recovery action', async () => {
    const { emitted } = render(CanvasChrome, {
      props: {
        isInert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: null,
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          isReadOnly: false,
          catalogRecovery: { status: 'reload-required', boardId: 'board-a' },
          archived: { isArchived: false, isRestoring: false },
          setNameInputRef: vi.fn(),
        },
        account: {
          balance: {
            status: 'ready',
            availableIdeas: 0,
            displayValue: '0',
            ariaLabel: 'Idea余额 0 Ideas',
            command: vi.fn(),
          },
          menuItems: [],
        },
        tools: {
          activeTool: 'select',
          isLibraryMounted: false,
          isLibraryOpen: false,
          isMinimapEnabled: false,
        },
        status: { isExportDisabled: false, zoomLevel: 100 },
      },
    })

    screen.getByRole('button', { name: 'Reload canvas list' }).click()

    expect(emitted()['reload-catalog']).toHaveLength(1)
  })

  it('starts Board rename from the keyboard before confirming the draft', async () => {
    const { emitted, user } = renderWithUser(CanvasChrome, {
      props: {
        isInert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: 'board-a',
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          isReadOnly: false,
          catalogRecovery: { status: 'idle' },
          archived: { isArchived: false, isRestoring: false },
          setNameInputRef: vi.fn(),
        },
        account: {
          balance: {
            status: 'ready',
            availableIdeas: 0,
            displayValue: '0',
            ariaLabel: 'Idea余额 0 Ideas',
            command: vi.fn(),
          },
          menuItems: [],
        },
        tools: {
          activeTool: 'select',
          isLibraryMounted: false,
          isLibraryOpen: false,
          isMinimapEnabled: false,
        },
        status: { isExportDisabled: false, zoomLevel: 100 },
      },
    })

    const name = screen.getByRole('textbox', { name: 'Canvas name' })
    name.focus()
    await user.keyboard('{Enter}')

    expect(emitted()['start-board-rename']).toHaveLength(1)
    expect(emitted()['confirm-board-rename']).toBeUndefined()
  })

  it('forwards Board rename draft, confirm, cancel, pending, and DOM ref behavior', async () => {
    const setNameInputRef = vi.fn()
    const { emitted, user, rerender } = renderWithUser(CanvasChrome, {
      props: {
        isInert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: 'board-a',
          isMenuOpen: false,
          isRenaming: true,
          isRenamePending: false,
          isReadOnly: false,
          catalogRecovery: { status: 'idle' },
          archived: { isArchived: false, isRestoring: false },
          setNameInputRef,
        },
        account: {
          balance: {
            status: 'ready',
            availableIdeas: 0,
            displayValue: '0',
            ariaLabel: 'Idea余额 0 Ideas',
            command: vi.fn(),
          },
          menuItems: [],
        },
        tools: {
          activeTool: 'select',
          isLibraryMounted: false,
          isLibraryOpen: false,
          isMinimapEnabled: false,
        },
        status: { isExportDisabled: false, zoomLevel: 100 },
      },
    })

    const name = screen.getByRole('textbox', { name: 'Canvas name' })
    expect(setNameInputRef).toHaveBeenLastCalledWith(name)
    expect(name).toHaveAttribute('contenteditable', 'plaintext-only')
    expect(name).toHaveAttribute('aria-readonly', 'false')

    name.textContent = 'Renamed'
    name.dispatchEvent(new Event('input', { bubbles: true }))
    name.focus()
    await user.keyboard('{Enter}')

    expect(emitted()['update:board-name-draft']).toEqual([['Renamed']])
    expect(emitted()['confirm-board-rename']).toHaveLength(1)

    await rerender({
      board: {
        currentName: 'Renamed',
        options: [],
        currentId: 'board-a',
        isMenuOpen: false,
        isRenaming: true,
        isRenamePending: true,
        isReadOnly: false,
        catalogRecovery: { status: 'idle' },
        archived: { isArchived: false, isRestoring: false },
        setNameInputRef,
      },
    })

    expect(name).toHaveAttribute('contenteditable', 'false')
    expect(name).toHaveAttribute('aria-readonly', 'true')
    expect(name).toHaveAttribute('aria-busy', 'true')
    expect(screen.getByText('Syncing name')).toBeVisible()

    await user.keyboard('{Escape}')
    expect(emitted()['cancel-board-rename']).toHaveLength(1)
  })

  it('keeps Board catalog selection and recovery retry keyboard-accessible', async () => {
    const { emitted, user } = renderWithUser(CanvasChrome, {
      props: {
        isInert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [
            { id: 'board-a', name: 'Untitled' },
            { id: 'board-b', name: 'Second Board' },
          ],
          currentId: 'board-a',
          isMenuOpen: true,
          isRenaming: false,
          isRenamePending: false,
          isReadOnly: false,
          catalogRecovery: { status: 'reload-required', boardId: 'board-a' },
          archived: { isArchived: false, isRestoring: false },
          setNameInputRef: vi.fn(),
        },
        account: {
          balance: {
            status: 'ready',
            availableIdeas: 0,
            displayValue: '0',
            ariaLabel: 'Idea余额 0 Ideas',
            command: vi.fn(),
          },
          menuItems: [],
        },
        tools: {
          activeTool: 'select',
          isLibraryMounted: false,
          isLibraryOpen: false,
          isMinimapEnabled: false,
        },
        status: { isExportDisabled: false, zoomLevel: 100 },
      },
    })

    const secondBoard = screen.getByRole('button', { name: 'Second Board' })
    secondBoard.focus()
    await user.keyboard('{Enter}')
    await user.click(screen.getByRole('button', { name: 'Reload canvas list' }))

    expect(emitted()['select-board']).toEqual([['board-b']])
    expect(emitted()['reload-catalog']).toHaveLength(1)
  })

  it('places an active Library control immediately above the minimap control', async () => {
    const { emitted, user } = renderWithUser(CanvasChrome, {
      props: {
        isInert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: null,
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          isReadOnly: false,
          catalogRecovery: { status: 'idle' },
          archived: { isArchived: false, isRestoring: false },
          setNameInputRef: vi.fn(),
        },
        account: {
          balance: {
            status: 'ready',
            availableIdeas: 0,
            displayValue: '0',
            ariaLabel: 'Idea余额 0 Ideas',
            command: vi.fn(),
          },
          menuItems: [],
        },
        tools: {
          activeTool: 'select',
          isLibraryMounted: true,
          isLibraryOpen: true,
          isMinimapEnabled: false,
        },
        status: { isExportDisabled: false, zoomLevel: 100 },
      },
    })

    const library = screen.getByRole('button', { name: 'Library' })
    const minimap = screen.getByRole('button', { name: 'MiniMap' })

    expect(library.nextElementSibling).toBe(minimap)
    expect(library).toHaveAttribute('aria-pressed', 'true')
    await user.click(library)
    expect(emitted()['toggle-library']).toHaveLength(1)
  })

  it('publishes a scoped focus state for contained Library inputs', () => {
    const { emitted } = render(CanvasChrome, {
      props: {
        isInert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: null,
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          isReadOnly: false,
          catalogRecovery: { status: 'idle' },
          archived: { isArchived: false, isRestoring: false },
          setNameInputRef: vi.fn(),
        },
        account: {
          balance: {
            status: 'ready',
            availableIdeas: 0,
            displayValue: '0',
            ariaLabel: 'Idea余额 0 Ideas',
            command: vi.fn(),
          },
          menuItems: [],
        },
        tools: {
          activeTool: 'select',
          isLibraryMounted: true,
          isLibraryOpen: true,
          isMinimapEnabled: false,
        },
        status: { isExportDisabled: false, zoomLevel: 100 },
      },
      slots: {
        library: () => h('input', { 'aria-label': 'Library folder name' }),
      },
    })

    const input = screen.getByRole('textbox', { name: 'Library folder name' })
    input.dispatchEvent(new FocusEvent('focusin', { bubbles: true }))
    input.dispatchEvent(new FocusEvent('focusout', { bubbles: true, relatedTarget: document.body }))

    expect(emitted()['library-focus-change']).toEqual([[true], [false]])
  })

  it('keeps the full Canvas Chrome inaccessible while hidden and inert', () => {
    const { container } = render(CanvasChrome, {
      props: {
        isInert: true,
        isHidden: true,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: null,
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          isReadOnly: false,
          catalogRecovery: { status: 'idle' },
          archived: { isArchived: false, isRestoring: false },
          setNameInputRef: vi.fn(),
        },
        account: {
          balance: {
            status: 'ready',
            availableIdeas: 0,
            displayValue: '0',
            ariaLabel: 'Idea余额 0 Ideas',
            command: vi.fn(),
          },
          menuItems: [],
        },
        tools: {
          activeTool: 'select',
          isLibraryMounted: false,
          isLibraryOpen: false,
          isMinimapEnabled: false,
        },
        status: { isExportDisabled: false, zoomLevel: 100 },
      },
    })

    const regions = container.querySelectorAll('.canvas-chrome-region')
    expect(regions).toHaveLength(3)
    for (const region of regions) {
      expect(region.getAttribute('class')).toContain('is-hidden')
      expect(region).toHaveAttribute('inert')
    }
  })

  it('hides the archived banner for live boards', () => {
    render(CanvasChrome, {
      props: {
        isInert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: 'board-a',
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          isReadOnly: false,
          catalogRecovery: { status: 'idle' },
          archived: { isArchived: false, isRestoring: false },
          setNameInputRef: vi.fn(),
        },
        account: {
          balance: {
            status: 'ready',
            availableIdeas: 0,
            displayValue: '0',
            ariaLabel: 'Idea余额 0 Ideas',
            command: vi.fn(),
          },
          menuItems: [],
        },
        tools: {
          activeTool: 'select',
          isLibraryMounted: false,
          isLibraryOpen: false,
          isMinimapEnabled: false,
        },
        status: { isExportDisabled: false, zoomLevel: 100 },
      },
    })

    expect(screen.queryByRole('button', { name: 'Restore archived board' })).toBeNull()
  })

  it('shows a non-dismissible archived banner with a restore action', async () => {
    const { emitted } = render(CanvasChrome, {
      props: {
        isInert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: 'board-a',
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          isReadOnly: false,
          catalogRecovery: { status: 'idle' },
          archived: { isArchived: true, isRestoring: false },
          setNameInputRef: vi.fn(),
        },
        account: {
          balance: {
            status: 'ready',
            availableIdeas: 0,
            displayValue: '0',
            ariaLabel: 'Idea余额 0 Ideas',
            command: vi.fn(),
          },
          menuItems: [],
        },
        tools: {
          activeTool: 'select',
          isLibraryMounted: false,
          isLibraryOpen: false,
          isMinimapEnabled: false,
        },
        status: { isExportDisabled: false, zoomLevel: 100 },
      },
    })

    // Non-dismissible: the banner carries the archived label with no close control.
    expect(screen.getByRole('status')).toBeVisible()
    const restore = screen.getByRole('button', { name: 'Restore archived board' })
    expect(restore).toBeEnabled()
    expect(screen.queryByRole('button', { name: 'Close' })).toBeNull()

    restore.click()
    expect(emitted()['restore-archived-board']).toHaveLength(1)
  })

  it('disables the restore action while a restore is in flight', () => {
    render(CanvasChrome, {
      props: {
        isInert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: 'board-a',
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          isReadOnly: false,
          catalogRecovery: { status: 'idle' },
          archived: { isArchived: true, isRestoring: true },
          setNameInputRef: vi.fn(),
        },
        account: {
          balance: {
            status: 'ready',
            availableIdeas: 0,
            displayValue: '0',
            ariaLabel: 'Idea余额 0 Ideas',
            command: vi.fn(),
          },
          menuItems: [],
        },
        tools: {
          activeTool: 'select',
          isLibraryMounted: false,
          isLibraryOpen: false,
          isMinimapEnabled: false,
        },
        status: { isExportDisabled: false, zoomLevel: 100 },
      },
    })

    expect(screen.getByRole('button', { name: 'Restore archived board' })).toBeDisabled()
  })

  describe('avatar workspace section', () => {
    function workspaceProps(overrides?: Record<string, unknown>) {
      const onSelect = vi.fn()
      const workspace = {
        items: [
          {
            publicId: 'ws_default1',
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
        selectedId: 'ws_default1',
        status: 'ready',
        onSelect,
        onCreate: vi.fn(),
        onRename: vi.fn(),
        onDelete: vi.fn(),
        onRetry: vi.fn(),
        ...overrides,
      }
      return { workspace, onSelect }
    }

    function chromePropsWithWorkspace(workspace: Record<string, unknown>) {
      return {
        isInert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: 'board-a',
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          isReadOnly: false,
          catalogRecovery: { status: 'idle' },
          archived: { isArchived: false, isRestoring: false },
          setNameInputRef: vi.fn(),
        },
        account: {
          balance: {
            status: 'ready',
            availableIdeas: 0,
            displayValue: '0',
            ariaLabel: 'Idea余额 0 Ideas',
            command: vi.fn(),
          },
          menuItems: [{ key: 'profile', label: 'Account', command: vi.fn() }],
          workspace,
        },
        tools: {
          activeTool: 'select',
          isLibraryMounted: false,
          isLibraryOpen: false,
          isMinimapEnabled: false,
        },
        status: { isExportDisabled: false, zoomLevel: 100 },
      }
    }

    it('renders the workspace row with the reused manage entry', async () => {
      const { workspace } = workspaceProps()
      const { user } = renderWithUser(CanvasChrome, {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        props: chromePropsWithWorkspace(workspace) as any,
      })

      await user.click(screen.getByRole('button', { name: 'Account menu' }))

      expect(await screen.findByRole('menuitem', { name: 'Default' })).toBeVisible()
      expect(screen.getByRole('button', { name: 'Manage workspaces' })).toBeVisible()
    })

    it('switches workspaces from the submenu and highlights the current row', async () => {
      const { workspace, onSelect } = workspaceProps()
      const { user } = renderWithUser(CanvasChrome, {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        props: chromePropsWithWorkspace(workspace) as any,
      })

      await user.click(screen.getByRole('button', { name: 'Account menu' }))
      const trigger = await screen.findByRole('menuitem', { name: 'Default' })
      trigger.dispatchEvent(
        new PointerEvent('pointermove', {
          bubbles: true,
          cancelable: true,
          pointerType: 'mouse',
        }),
      )

      const teamOption = await screen.findByRole('menuitem', { name: 'Team' })
      expect(teamOption).toBeVisible()

      const currentOption = (await screen.findAllByRole('menuitem', { name: 'Default' })).find(
        (entry) => entry.getAttribute('aria-current') === 'true',
      )
      expect(currentOption).toBeDefined()

      await user.click(teamOption)
      await waitFor(() => expect(onSelect).toHaveBeenCalledWith('ws_team2'))
      await waitFor(() =>
        expect(screen.getByRole('button', { name: 'Account menu' })).toHaveAttribute(
          'aria-expanded',
          'false',
        ),
      )
    })
  })
})
