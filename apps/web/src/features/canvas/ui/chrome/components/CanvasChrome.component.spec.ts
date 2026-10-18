import { describe, expect, it, vi } from 'vitest'

vi.mock('../../../../../core/composables/useSupportChannels', () => ({
  useSupportChannels: () => ({ channels: [] }),
}))
import { h } from 'vue'
import { render, renderWithUser, screen } from '../../../../../test/render'
import CanvasChrome from './CanvasChrome.vue'

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
})
