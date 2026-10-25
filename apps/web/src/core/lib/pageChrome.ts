function joinClassNames(...classes: Array<string | false | null | undefined>) {
  return classes.filter(Boolean).join(' ')
}

export const pageChromeClasses = {
  roundControl: 'round-control page-chrome-control',
  icon: 'page-chrome-icon',
} as const

export const pageChromeAccountMenuClasses = {
  wrap: 'relative page-chrome-account',
  avatarButton: `${pageChromeClasses.roundControl} ib-user-avatar-button`,
  avatarImage: 'ib-user-avatar-button__image',
  avatarPlaceholder: 'ib-user-avatar-button__placeholder',
  avatarIcon: 'size-5',
} as const

export function createPageChromeRoundControlClass(...extraClasses: string[]) {
  return joinClassNames(pageChromeClasses.roundControl, ...extraClasses)
}

export function createPageChromeAccountRoundControlClass(...extraClasses: string[]) {
  return joinClassNames(pageChromeAccountMenuClasses.avatarButton, ...extraClasses)
}
