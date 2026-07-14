import { describe, expect, it, vi } from 'vitest'
import { render, renderWithUser, screen } from '../../test/render'
import UserAccountMenu from './UserAccountMenu.vue'
import type { UserAccountMenuItem, UserIdeaBalanceControl } from './userAccountMenu.types'

describe('UserAccountMenu', () => {
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
    const accountMenu = screen.getByRole('button', { name: '账户菜单' })

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
})
