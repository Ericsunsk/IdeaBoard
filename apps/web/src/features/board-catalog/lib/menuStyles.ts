function createBoardPopupMenuPt(rootClass: string, itemLinkClass = '!p-0') {
  return {
    root: { class: rootClass },
    list: { class: '!m-0 !flex !flex-col !gap-0.5 !p-1.5' },
    itemContent: { class: '!rounded-lg' },
    itemLink: { class: itemLinkClass },
  } as const
}

const boardPopupMenuThemeClass =
  '[--app-overlay-menu-item-color:var(--color-board-menu-item)] [--app-overlay-menu-item-hover-bg:var(--color-board-menu-item-hover-bg)] [--app-overlay-menu-item-hover-color:var(--color-board-menu-item)] [--app-overlay-menu-danger-color:var(--color-board-menu-danger)] [--app-overlay-menu-danger-hover-bg:var(--color-board-menu-danger-hover-bg)] [--app-overlay-menu-danger-hover-color:var(--color-board-menu-danger)]'

const boardsDropdownRootClass = `!z-40 !w-32 !min-w-[8rem] !max-w-[8rem] !rounded-[var(--radius-ib-control)] !border !border-board-menu-border !bg-board-overlay-bg !p-0 !shadow-none backdrop-blur-[10px] ${boardPopupMenuThemeClass}`

export const boardMenuPt = createBoardPopupMenuPt(boardsDropdownRootClass, '')
