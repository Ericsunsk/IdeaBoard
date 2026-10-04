import { describe, expect, it, vi } from 'vitest'
import { render, renderWithUser, screen, waitFor } from '../../test/render'
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

  it('renders before-balance slot leading element before credit balance in authenticated state', () => {
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
        'before-balance': '<button type="button" aria-label="locale-trigger">Translate</button>',
        actions: '<button type="button" aria-label="custom-action">Custom Action</button>',
      },
    })

    const localeTrigger = screen.getByRole('button', { name: 'locale-trigger' })
    const credits = screen.getByRole('button', { name: 'Idea余额 100 Ideas' })
    const customAction = screen.getByRole('button', { name: 'custom-action' })
    const accountMenu = screen.getByRole('button', { name: 'Account menu' })

    expect(localeTrigger).toBeVisible()
    expect(localeTrigger.nextElementSibling).toBe(credits)
    expect(credits.nextElementSibling).toBe(customAction)
    expect(customAction.nextElementSibling).toBe(accountMenu)
  })

  it('renders before-balance slot leading element before guest login and register buttons in guest state', () => {
    const items: UserAccountMenuItem[] = [
      { key: 'login', label: '登录', command: vi.fn() },
      { key: 'register', label: '免费注册', command: vi.fn() },
    ]
    render(UserAccountMenu, {
      props: { items },
      slots: {
        'before-balance': '<button type="button" aria-label="locale-trigger">Translate</button>',
      },
    })

    const localeTrigger = screen.getByRole('button', { name: 'locale-trigger' })
    const loginBtn = screen.getByRole('button', { name: 'Sign in' })
    const registerBtn = screen.getByRole('button', { name: 'Sign up for free' })

    expect(localeTrigger).toBeVisible()
    expect(localeTrigger.nextElementSibling).toBe(loginBtn)
    expect(loginBtn.nextElementSibling).toBe(registerBtn)
  })

  it('renders appearance sub-menu allowing tri-state theme switching', async () => {
    const items: UserAccountMenuItem[] = [{ key: 'profile', label: '账户管理', command: vi.fn() }]
    const { user } = renderWithUser(UserAccountMenu, { props: { items } })

    const trigger = screen.getByRole('button', { name: 'Account menu' })
    await user.click(trigger)

    const appearanceSpan = await screen.findByText('Appearance')
    expect(appearanceSpan).toBeVisible()

    await user.click(appearanceSpan)

    expect(await screen.findByRole('menuitem', { name: /Light/i })).toBeVisible()
    expect(screen.getByRole('menuitem', { name: /Dark/i })).toBeVisible()
    expect(screen.getByRole('menuitem', { name: /System/i })).toBeVisible()
  })
})
