import { describe, expect, it, vi } from 'vitest'
import { render, renderWithUser, screen } from '../../../../test/render'
import CanvasChrome from './CanvasChrome.vue'

describe('CanvasChrome', () => {
  it('keeps only accessible viewport zoom controls without autosave feedback controls', () => {
    render(CanvasChrome, {
      props: {
        inert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: null,
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          catalogRecovery: { status: 'idle' },
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
        tools: { activeTool: 'select', minimapEnabled: false },
        status: { exportDisabled: false, zoomLevel: 100 },
      },
    })

    expect(screen.queryByText('保存中')).toBeNull()
    expect(screen.queryByText('恢复保存中')).toBeNull()
    expect(screen.queryByText('远端未同步')).toBeNull()
    expect(screen.queryByText('保存冲突')).toBeNull()
    expect(screen.queryByText('未保存')).toBeNull()
    expect(screen.queryByRole('button', { name: '重试画布自动保存' })).toBeNull()
    expect(screen.getByRole('button', { name: '缩小' })).toBeVisible()
    expect(screen.getByText('100%')).toBeVisible()
    expect(screen.getByRole('button', { name: '放大' })).toBeVisible()
  })

  it('places the credits balance immediately before the share control', () => {
    render(CanvasChrome, {
      props: {
        inert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: null,
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          catalogRecovery: { status: 'idle' },
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
        tools: { activeTool: 'select', minimapEnabled: false },
        status: { exportDisabled: false, zoomLevel: 100 },
      },
    })

    const credits = screen.getByRole('button', { name: 'Idea余额 100 Ideas' })
    const share = screen.getByRole('button', { name: '分享功能稍后接入' })

    expect(share.previousElementSibling).toBe(credits)
    expect(share).toBeDisabled()
    expect(share).toHaveAttribute('title', '分享功能稍后接入')
    expect(screen.getAllByRole('button', { name: 'Idea余额 100 Ideas' })).toHaveLength(1)
    expect(credits).toHaveTextContent('100')
    expect(credits).not.toHaveTextContent('Ideas')
  })

  it('exposes an executable catalog reload recovery action', async () => {
    const { emitted } = render(CanvasChrome, {
      props: {
        inert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: null,
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          catalogRecovery: { status: 'reload-required', boardId: 'board-a' },
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
        tools: { activeTool: 'select', minimapEnabled: false },
        status: { exportDisabled: false, zoomLevel: 100 },
      },
    })

    screen.getByRole('button', { name: '重新加载画布列表' }).click()

    expect(emitted()['reload-board-catalog']).toHaveLength(1)
  })

  it('starts Board rename from the keyboard before confirming the draft', async () => {
    const { emitted, user } = renderWithUser(CanvasChrome, {
      props: {
        inert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: 'board-a',
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          catalogRecovery: { status: 'idle' },
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
        tools: { activeTool: 'select', minimapEnabled: false },
        status: { exportDisabled: false, zoomLevel: 100 },
      },
    })

    const name = screen.getByRole('textbox', { name: '画布名称' })
    name.focus()
    await user.keyboard('{Enter}')

    expect(emitted()['start-board-rename']).toHaveLength(1)
    expect(emitted()['confirm-board-rename']).toBeUndefined()
  })

  it('forwards Board rename draft, confirm, cancel, pending, and DOM ref behavior', async () => {
    const setNameInputRef = vi.fn()
    const { emitted, user, rerender } = renderWithUser(CanvasChrome, {
      props: {
        inert: false,
        isHidden: false,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: 'board-a',
          isMenuOpen: false,
          isRenaming: true,
          isRenamePending: false,
          catalogRecovery: { status: 'idle' },
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
        tools: { activeTool: 'select', minimapEnabled: false },
        status: { exportDisabled: false, zoomLevel: 100 },
      },
    })

    const name = screen.getByRole('textbox', { name: '画布名称' })
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
        catalogRecovery: { status: 'idle' },
        setNameInputRef,
      },
    })

    expect(name).toHaveAttribute('contenteditable', 'false')
    expect(name).toHaveAttribute('aria-readonly', 'true')
    expect(name).toHaveAttribute('aria-busy', 'true')
    expect(screen.getByText('名称同步中')).toBeVisible()

    await user.keyboard('{Escape}')
    expect(emitted()['cancel-board-rename']).toHaveLength(1)
  })

  it('keeps Board catalog selection and recovery retry keyboard-accessible', async () => {
    const { emitted, user } = renderWithUser(CanvasChrome, {
      props: {
        inert: false,
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
          catalogRecovery: { status: 'reload-required', boardId: 'board-a' },
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
        tools: { activeTool: 'select', minimapEnabled: false },
        status: { exportDisabled: false, zoomLevel: 100 },
      },
    })

    const secondBoard = screen.getByRole('button', { name: 'Second Board' })
    secondBoard.focus()
    await user.keyboard('{Enter}')
    await user.click(screen.getByRole('button', { name: '重新加载画布列表' }))

    expect(emitted()['select-board']).toEqual([['board-b']])
    expect(emitted()['reload-board-catalog']).toHaveLength(1)
  })

  it('keeps the full Canvas Chrome inaccessible while hidden and inert', () => {
    const { container } = render(CanvasChrome, {
      props: {
        inert: true,
        isHidden: true,
        board: {
          currentName: 'Untitled',
          options: [],
          currentId: null,
          isMenuOpen: false,
          isRenaming: false,
          isRenamePending: false,
          catalogRecovery: { status: 'idle' },
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
        tools: { activeTool: 'select', minimapEnabled: false },
        status: { exportDisabled: false, zoomLevel: 100 },
      },
    })

    const regions = container.querySelectorAll('.canvas-chrome-region')
    expect(regions).toHaveLength(3)
    for (const region of regions) {
      expect(region.getAttribute('class')).toContain('is-hidden')
      expect(region).toHaveAttribute('inert')
    }
  })
})
