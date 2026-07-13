import { describe, expect, it, vi } from 'vitest'
import { render, renderWithUser, screen } from '../../test/render'
import UserAccountMenu from './UserAccountMenu.vue'
import type { UserAccountMenuItem } from './userAccountMenuActions'

describe('UserAccountMenu', () => {
  it('renders credits as a dedicated balance control before the avatar', async () => {
    const openCredits = vi.fn()
    const items: UserAccountMenuItem[] = [
      { key: 'profile', label: '账户管理', command: vi.fn() },
      { key: 'credits', label: 'Idea', meta: '1,280', command: openCredits },
      { key: 'logout', label: '退出登录', command: vi.fn() },
    ]
    const { user } = renderWithUser(UserAccountMenu, { props: { items } })

    const credits = screen.getByRole('button', { name: 'Idea余额 1,280' })
    const accountMenu = screen.getByRole('button', { name: '账户菜单' })

    expect(credits).toBeVisible()
    expect(credits.querySelector('svg')).not.toBeNull()
    expect(credits.nextElementSibling).toBe(accountMenu)

    await user.click(accountMenu)
    expect(await screen.findByText('账户管理')).toBeVisible()
    expect(screen.queryByText('Idea')).toBeNull()

    await user.click(credits)
    expect(openCredits).toHaveBeenCalledOnce()
  })

  it('keeps the balance control stable while credits are loading', () => {
    const items: UserAccountMenuItem[] = [
      { key: 'profile', label: '账户管理', command: vi.fn() },
      { key: 'credits', label: 'Idea', meta: '加载中', command: vi.fn() },
    ]

    render(UserAccountMenu, { props: { items } })

    const credits = screen.getByRole('button', { name: 'Idea余额 加载中' })
    expect(credits).toHaveAttribute('aria-busy', 'true')
    expect(credits).toHaveTextContent('加载中')
  })
})
