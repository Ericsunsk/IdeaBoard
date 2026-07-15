import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '../../../../test/render'
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
})
