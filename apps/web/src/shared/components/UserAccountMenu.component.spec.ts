import { beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '../i18n'
import { render, renderWithUser, screen, waitFor } from '../../test/render'
import UserAccountMenu from './UserAccountMenu.vue'
import type { UserAccountMenuItem, UserIdeaBalanceControl } from './userAccountMenu.types'

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
  beforeEach(() => {
    i18n.global.locale.value = 'en'
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
    expect(
      email.compareDocumentPosition(separator!) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
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
  })

  it('places configured support channels after language and links the QR code safely', async () => {
    const supportChannels: import('../composables/useSupportChannels').SupportChannel[] = [
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
    expect(
      language.compareDocumentPosition(support) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
    expect(
      support.compareDocumentPosition(signOutSeparator!) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
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
    // One separator after email, one separator before danger item (logout)
    expect(separators).toHaveLength(2)

    // The first separator should immediately follow the email label container
    const emailContainer = emailLabel.closest('div') ?? emailLabel
    expect(emailContainer.nextElementSibling).toBe(separators[0])

    // There should be no separator directly between profile and appearance
    const profileItem = screen.getByRole('menuitem', { name: '账户管理' })
    const appearanceTrigger = screen.getByRole('menuitem', { name: /Appearance/i })
    expect(profileItem.nextElementSibling).toBe(appearanceTrigger)
  })
})
