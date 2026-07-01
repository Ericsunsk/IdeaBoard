export type PrimePopupMenuRef = {
  toggle: (event: Event, target?: HTMLElement) => void
  hide: () => void
  container?: HTMLElement | null
}

function createBoardPopupMenuPt(rootClass: string, itemLinkClass = '!p-0') {
  return {
    root: { class: rootClass },
    list: { class: '!m-0 !flex !flex-col !gap-0.5 !p-1.5' },
    itemContent: { class: '!rounded-lg' },
    itemLink: { class: itemLinkClass },
  } as const
}

const boardsDropdownRootClass =
  '!z-40 !w-32 !min-w-[8rem] !max-w-[8rem] !rounded-[var(--radius-ib-control)] !border !border-board-menu-border !bg-board-overlay-bg !p-0 !shadow-none backdrop-blur-[10px]'

export const userMenuPt = createBoardPopupMenuPt(boardsDropdownRootClass, '')
export const boardMenuPt = userMenuPt

export const popupMenuItemClass =
  'flex w-full items-center rounded-lg px-2.5 py-2 text-[13px] font-semibold text-board-menu-item no-underline transition-colors hover:bg-board-menu-item-hover-bg'
export const popupMenuDangerItemClass =
  '!text-board-menu-danger hover:!bg-board-menu-danger-hover-bg'
