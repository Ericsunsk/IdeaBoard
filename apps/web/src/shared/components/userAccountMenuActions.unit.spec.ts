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
})
