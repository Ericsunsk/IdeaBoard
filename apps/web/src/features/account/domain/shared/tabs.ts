export type AccountTab =
  'profile' | 'security' | 'recharge' | 'subscriptions' | 'rewards' | 'billing' | 'usage'

export const defaultAccountTab: AccountTab = 'profile'

const accountTabKeys = [
  'profile',
  'security',
  'recharge',
  'subscriptions',
  'rewards',
  'billing',
  'usage',
] as const

function isAccountTab(value: unknown): value is AccountTab {
  return typeof value === 'string' && (accountTabKeys as readonly string[]).includes(value)
}

export function resolveAccountTabKey(value: string | undefined): AccountTab {
  if (isAccountTab(value)) {
    return value
  }
  return defaultAccountTab
}
