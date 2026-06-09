export type PrimePopupMenuRef = {
  toggle: (event: Event) => void
  hide: () => void
}

export const boardMenuPt = {
  root: {
    class:
      '!z-30 !min-w-[118px] !rounded-[10px] !border !border-[#d8ddea] !bg-white !p-0 !shadow-none',
  },
  list: { class: '!m-0 !flex !flex-col !gap-0.5 !p-1.5' },
  itemContent: { class: '!rounded-lg' },
  itemLink: { class: '!p-0' },
} as const

export const userMenuPt = {
  root: {
    class:
      '!z-40 !w-32 !rounded-[10px] !border !border-[#d8ddea] !bg-white/98 !p-0 !shadow-none backdrop-blur-[10px]',
  },
  list: { class: '!m-0 !flex !flex-col !gap-0.5 !p-1.5' },
  itemContent: { class: '!rounded-lg' },
  itemLink: { class: '!p-0' },
} as const

export const boardMenuItemClass =
  'flex h-8 w-full items-center rounded-lg px-2.5 text-[13px] font-semibold no-underline transition-colors hover:bg-[#eff3fb]'
export const boardMenuDangerItemClass = 'text-[#c23b42] hover:bg-[#fdeced]'
export const boardMenuDefaultItemClass = 'text-[#3f475d]'
