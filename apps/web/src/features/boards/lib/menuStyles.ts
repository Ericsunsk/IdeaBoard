export type PrimePopupMenuRef = {
  toggle: (event: Event) => void
  hide: () => void
}

function createBoardPopupMenuPt(rootClass: string, itemLinkClass = '!p-0') {
  return {
    root: { class: rootClass },
    list: { class: '!m-0 !flex !flex-col !gap-0.5 !p-1.5' },
    itemContent: { class: '!rounded-lg' },
    itemLink: { class: itemLinkClass },
  } as const
}

export const boardMenuPt = createBoardPopupMenuPt(
  '!z-30 !min-w-[118px] !rounded-[10px] !border !border-[#d8ddea] !bg-white !p-0 !shadow-none',
)

export const userMenuPt = createBoardPopupMenuPt(
  '!z-40 !w-32 !min-w-[8rem] !max-w-[8rem] !rounded-[10px] !border !border-[#d8ddea] !bg-white/98 !p-0 !shadow-none backdrop-blur-[10px]',
  '',
)

export const boardMenuItemClass =
  'flex h-8 w-full items-center rounded-lg px-2.5 text-[13px] font-semibold no-underline transition-colors hover:bg-[#eff3fb]'
export const boardMenuDangerItemClass = 'text-[#c23b42] hover:bg-[#fdeced]'
export const boardMenuDefaultItemClass = 'text-[#3f475d]'
