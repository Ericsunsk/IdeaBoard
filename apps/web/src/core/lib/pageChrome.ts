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
