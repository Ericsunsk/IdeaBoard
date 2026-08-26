function joinClassNames(...classes: Array<string | false | null | undefined>) {
  return classes.filter(Boolean).join(' ')
}

const pageChromeClasses = {
  roundControl: 'ib-round-control ib-page-chrome-control',
} as const

export const pageChromeAccountMenuClasses = {
  wrap: 'relative ib-page-chrome-account',
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
