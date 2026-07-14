export type UserAccountMenuItemKey = 'profile' | 'admin' | 'home' | 'logout'

export type UserAccountMenuItemTone = 'default' | 'danger'

export type UserAccountMenuItem = {
  key: UserAccountMenuItemKey
  label: string
  tone?: UserAccountMenuItemTone
  command: () => void
}

type UserIdeaBalanceControlBase = {
  displayValue: string
  ariaLabel: string
  command: () => void
}

export type UserIdeaBalanceControl =
  | (UserIdeaBalanceControlBase & { status: 'loading' })
  | (UserIdeaBalanceControlBase & { status: 'ready'; availableIdeas: number })
  | (UserIdeaBalanceControlBase & { status: 'error'; errorMessage: string })
