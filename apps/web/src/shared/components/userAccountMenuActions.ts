import { formatIdeaAmount } from '../utils/ideaCurrency'

export type UserAccountMenuItemKey = 'profile' | 'credits' | 'admin' | 'home' | 'logout'

export type UserAccountMenuItemTone = 'default' | 'danger'

export type UserAccountMenuItem = {
  key: UserAccountMenuItemKey
  label: string
  meta?: string
  tone?: UserAccountMenuItemTone
  command: () => void
}

export type CreateUserAccountMenuItemsOptions = {
  canAccessAdmin: boolean
  hiddenKeys?: readonly UserAccountMenuItemKey[]
  availableCredits?: number
  handlers: {
    profile: () => void
    credits?: () => void
    admin?: () => void
    home?: () => void
    logout: () => void
  }
}

export function createUserAccountMenuItems(
  options: CreateUserAccountMenuItemsOptions,
): UserAccountMenuItem[] {
  const hiddenKeys = new Set(options.hiddenKeys ?? [])
  const items: UserAccountMenuItem[] = [
    {
      key: 'profile',
      label: '账户管理',
      command: options.handlers.profile,
    },
  ]

  if (options.handlers.credits) {
    items.push({
      key: 'credits',
      label: 'Idea',
      meta:
        options.availableCredits == null ? '加载中' : formatIdeaAmount(options.availableCredits),
      command: options.handlers.credits,
    })
  }

  if (options.canAccessAdmin && options.handlers.admin) {
    items.push({
      key: 'admin',
      label: '管理后台',
      command: options.handlers.admin,
    })
  }

  if (options.handlers.home) {
    items.push({
      key: 'home',
      label: '返回首页',
      command: options.handlers.home,
    })
  }

  items.push({
    key: 'logout',
    label: '退出登录',
    tone: 'danger',
    command: options.handlers.logout,
  })

  return items.filter((item) => !hiddenKeys.has(item.key))
}
