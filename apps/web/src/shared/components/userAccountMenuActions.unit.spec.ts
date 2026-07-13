import { describe, expect, it, vi } from 'vitest'
import { canAccessAdmin } from '../auth/adminAccess'
import { createUserAccountMenuItems } from './userAccountMenuActions'

describe('user account menu actions', () => {
  it('keeps regular user menus free of admin-only actions', () => {
    const items = createUserAccountMenuItems({
      canAccessAdmin: canAccessAdmin({ role: 'user' }),
      handlers: {
        profile: vi.fn(),
        admin: vi.fn(),
        home: vi.fn(),
        logout: vi.fn(),
      },
    })

    expect(items.map((item) => item.key)).toEqual(['profile', 'home', 'logout'])
  })

  it('adds admin route between profile and home for admins', () => {
    const items = createUserAccountMenuItems({
      canAccessAdmin: canAccessAdmin({ role: 'admin' }),
      handlers: {
        profile: vi.fn(),
        admin: vi.fn(),
        home: vi.fn(),
        logout: vi.fn(),
      },
    })

    expect(items.map((item) => item.key)).toEqual(['profile', 'admin', 'home', 'logout'])
    expect(items.map((item) => item.label)).toEqual([
      '账户管理',
      '管理后台',
      '返回首页',
      '退出登录',
    ])
  })

  it('allows surfaces to hide routes they do not expose', () => {
    const items = createUserAccountMenuItems({
      canAccessAdmin: true,
      hiddenKeys: ['home'],
      handlers: {
        profile: vi.fn(),
        admin: vi.fn(),
        home: vi.fn(),
        logout: vi.fn(),
      },
    })

    expect(items.map((item) => item.key)).toEqual(['profile', 'admin', 'logout'])
  })

  it('provides the current balance for the dedicated credits control', () => {
    const openCredits = vi.fn()
    const items = createUserAccountMenuItems({
      canAccessAdmin: false,
      availableCredits: 1280,
      handlers: {
        profile: vi.fn(),
        credits: openCredits,
        logout: vi.fn(),
      },
    })

    const credits = items.find((item) => item.key === 'credits')
    expect(credits).toMatchObject({ label: 'Idea', meta: '1,280' })
    credits?.command()
    expect(openCredits).toHaveBeenCalledOnce()
  })

  it('keeps very large balances compact inside the credits control', () => {
    const items = createUserAccountMenuItems({
      canAccessAdmin: false,
      availableCredits: 9_000_000_000_000_000,
      handlers: {
        profile: vi.fn(),
        credits: vi.fn(),
        logout: vi.fn(),
      },
    })

    expect(items.find((item) => item.key === 'credits')).toMatchObject({
      label: 'Idea',
      meta: '9000万亿',
    })
  })
})
