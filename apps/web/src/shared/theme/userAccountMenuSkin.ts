import type { MenuPassThroughOptions } from 'primevue/menu'

const userAccountMenuThemeClass =
  '[--app-overlay-menu-item-color:var(--color-ib-account-menu-item)] [--app-overlay-menu-item-hover-bg:var(--color-ib-account-menu-item-hover-bg)] [--app-overlay-menu-item-hover-color:var(--color-ib-account-menu-item)] [--app-overlay-menu-danger-color:var(--color-ib-account-menu-danger)] [--app-overlay-menu-danger-hover-bg:var(--color-ib-account-menu-danger-hover-bg)] [--app-overlay-menu-danger-hover-color:var(--color-ib-account-menu-danger)]'

const userAccountMenuRootClass = `!z-40 !mt-[10px] !w-32 !min-w-[8rem] !max-w-[8rem] !rounded-[var(--radius-ib-control)] !border !border-[var(--color-ib-account-menu-border)] !bg-[var(--color-ib-account-menu-bg)] !p-0 !shadow-none backdrop-blur-[10px] ${userAccountMenuThemeClass}`

export const userAccountMenuPt = {
  root: { class: userAccountMenuRootClass },
  list: { class: '!m-0 !flex !flex-col !gap-0.5 !p-1.5' },
  itemContent: { class: '!rounded-lg' },
  itemLink: { class: '' },
} satisfies MenuPassThroughOptions
