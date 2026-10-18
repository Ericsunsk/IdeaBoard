export const blankBoardPreviewSurfaceClass =
  'bg-[radial-gradient(circle,var(--color-page-dot)_1px,transparent_1px),linear-gradient(var(--color-board-card-preview-from),var(--color-board-card-preview-to))] [background-size:12px_12px,cover]'

export const catalogClasses = {
  page: 'app-scrollbar relative h-full w-full overflow-y-auto overflow-x-hidden overscroll-contain bg-background px-[var(--spacing-page-chrome-inset)] pt-[var(--spacing-page-chrome-inset)] pb-10 text-foreground [box-sizing:border-box] [-webkit-overflow-scrolling:touch] md:pb-16',
  pageBackground: 'ib-board-page-background page-dot-bg',
  header: 'relative z-[1] flex flex-wrap items-center justify-between gap-x-4',
  brand: 'inline-flex min-w-0 max-w-[calc(100%-64px)] items-center',
  main: 'relative z-[1] mx-auto mt-[46px] max-w-[1080px]',
  pageTitle:
    'm-0 break-words text-center text-[40px] leading-[1.05] font-bold tracking-normal md:text-[62px]',
  pageSubtitle:
    'mt-4 mb-[34px] text-center text-admin-title-sm tracking-normal text-muted-foreground md:text-[19px]',
  toolbar: 'mb-6 grid grid-cols-1 items-center gap-3.5 md:grid-cols-[auto_minmax(0,1fr)_auto_auto]',
  recoveryState:
    'mb-4 flex min-h-10 items-center justify-between gap-3 rounded-panel border border-border bg-card px-3 py-2',
  recoveryText: 'min-w-0 text-admin-body font-semibold text-foreground',
  recoveryButton:
    'inline-flex h-8 shrink-0 cursor-pointer items-center justify-center rounded-pill border-0 bg-primary px-3 text-xs font-bold text-primary-foreground hover:bg-primary-hover disabled:cursor-wait disabled:opacity-60',
  createButton:
    'cursor-pointer !h-[42px] !rounded-pill !border-0 !bg-primary !px-5 !text-sm !font-bold !tracking-normal !text-primary-foreground !shadow-none hover:!bg-primary-hover focus-visible:!ring-2 focus-visible:!ring-ring !w-full md:!w-auto',
  searchWrap: 'page-chrome-surface flex h-[42px] min-w-0 items-center gap-2 rounded-pill p-1.5',
  searchIcon: 'ml-2 size-3.5 shrink-0 text-muted-foreground',
  searchInput:
    'min-h-8 min-w-0 flex-1 bg-transparent px-1 text-admin-body text-foreground outline-none placeholder:text-muted-foreground',
  searchClearButton:
    'grid size-7 shrink-0 cursor-pointer place-items-center rounded-pill bg-muted text-muted-foreground transition-[background-color,color,transform] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:scale-105 hover:bg-border hover:text-foreground active:scale-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring mr-0.5',
  sortSelectTrigger:
    'page-chrome-surface group inline-flex h-[42px] w-full min-w-[108px] cursor-pointer items-center justify-between gap-2 rounded-pill border-[var(--color-page-chrome-border)] bg-[var(--color-page-chrome-bg)] px-3.5 text-admin-body font-semibold text-foreground backdrop-blur-[var(--blur-glass)] transition-[border-color,background-color,transform,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-[var(--color-page-chrome-border-hover)] hover:bg-[var(--color-page-chrome-bg-hover)] active:scale-[0.97] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring md:w-[108px]',
  sortSelectIcon:
    'size-3.5 shrink-0 text-muted-foreground transition-transform duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] group-data-[state=open]:rotate-180',
  sortSelectContent:
    'ib-user-account-menu-panel w-32 min-w-32 max-w-[8rem] rounded-panel border border-border bg-popover p-1',
  sortSelectItem:
    'control-state flex w-full cursor-pointer select-none items-center rounded-control px-2.5 py-2 text-admin-body font-semibold text-[var(--app-overlay-menu-item-color,var(--muted-foreground))] no-underline transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-[var(--app-overlay-menu-item-hover-bg,var(--accent-hover))] hover:text-[var(--app-overlay-menu-item-hover-color,var(--foreground))] focus:bg-[var(--app-overlay-menu-item-hover-bg,var(--accent-hover))] focus:text-[var(--app-overlay-menu-item-hover-color,var(--foreground))] data-[state=checked]:bg-muted data-[state=checked]:text-foreground data-[state=checked]:font-semibold',
  rangeToggleButtonActive:
    'border-primary bg-primary font-bold text-primary-foreground hover:bg-primary-hover hover:text-primary-foreground',
  rangeToggleIcon: 'size-5 shrink-0',
  accessBadge: 'mt-1 flex items-center gap-1 text-xs text-muted-foreground',
  accessBadgeIcon: 'size-3.5 shrink-0',
  loadingState:
    'flex min-h-[244px] items-center justify-center rounded-panel border border-dashed border-border bg-muted/30',
  loadingSpinner: '!h-9 !w-9 text-primary',
  boardsGrid: 'grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3.5',
  createCard:
    'relative flex h-[244px] w-full flex-col cursor-pointer overflow-hidden rounded-panel border border-border bg-card p-0 text-left transition-[border-color,transform] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-border/80 hover:-translate-y-0.5 hover:scale-[1.006] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
  createPreview: `flex h-[186px] w-full items-center justify-center border-b border-border ${blankBoardPreviewSurfaceClass}`,
  createPreviewIcon:
    "relative flex size-[54px] items-center justify-center rounded-panel border border-border bg-card/80 before:absolute before:top-1/2 before:left-1/2 before:h-[25px] before:w-[2.5px] before:-translate-x-1/2 before:-translate-y-1/2 before:rounded-pill before:bg-muted-foreground before:content-[''] after:absolute after:top-1/2 after:left-1/2 after:h-[2.5px] after:w-[25px] after:-translate-x-1/2 after:-translate-y-1/2 after:rounded-pill after:bg-muted-foreground after:content-['']",
  createMeta: 'px-3 pt-2.5 pb-3',
  createTitle: 'text-sm font-bold text-foreground',
  createDate: 'mt-[3px] text-xs text-muted-foreground',
  emptyTip: 'mt-[18px] text-admin-body text-muted-foreground',
  deleteDialogShell: 'p-5',
  deleteDialogTitle:
    'm-0 text-admin-heading leading-[1.2] font-bold tracking-normal text-foreground',
  deleteDialogDescription: 'mt-3 mb-0 text-admin-body leading-6 text-muted-foreground',
  deleteDialogFooter: 'mt-6 flex justify-end gap-2',
  deleteCancelButton:
    'inline-flex h-9 min-w-[72px] cursor-pointer items-center justify-center rounded-pill border-0 bg-transparent px-3.5 text-sm font-bold text-muted-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60',
  deleteConfirmButton:
    'inline-flex h-9 min-w-[72px] cursor-pointer items-center justify-center rounded-pill border-0 bg-destructive px-3.5 text-sm font-bold text-destructive-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-destructive-hover disabled:cursor-not-allowed disabled:opacity-70',
} as const
// UiDialog 以 :class/cn() 组合造型：mask 沿用 UiDialog 内置 ibDialogMaskClass。
export const catalogDeleteDialogContentClass =
  'overflow-hidden rounded-overlay border border-border bg-popover shadow-[var(--shadow-board-dialog)]'

export const catalogDeleteDialogStyle = {
  width: '360px',
  maxWidth: 'calc(100vw - 28px)',
} as const
