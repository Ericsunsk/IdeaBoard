import { blankBoardPreviewSurfaceClass } from '../../infrastructure/preview'

export { blankBoardPreviewSurfaceClass }

export const catalogClasses = {
  page: 'app-scrollbar relative h-full w-full overflow-y-auto overflow-x-hidden overscroll-contain bg-background px-[var(--spacing-ib-page-chrome-inset)] pt-[var(--spacing-ib-page-chrome-inset)] pb-10 text-foreground [box-sizing:border-box] [-webkit-overflow-scrolling:touch] md:pb-16',
  pageBackground: 'ib-board-page-background ib-page-dot-bg',
  header: 'relative z-[1] flex flex-wrap items-center justify-between gap-x-4',
  brand: 'inline-flex min-w-0 max-w-[calc(100%-64px)] items-center',
  main: 'relative z-[1] mx-auto mt-[46px] max-w-[1080px]',
  pageTitle:
    'm-0 break-words text-center text-[40px] leading-[1.05] font-bold tracking-normal md:text-[62px]',
  pageSubtitle:
    'mt-4 mb-[34px] text-center text-ib-title-sm tracking-normal text-muted-foreground md:text-[19px]',
  toolbar: 'mb-6 grid grid-cols-1 items-center gap-3.5 md:grid-cols-[auto_minmax(0,1fr)_auto]',
  recoveryState:
    'mb-4 flex min-h-10 items-center justify-between gap-3 rounded-ib-panel border border-border bg-card px-3 py-2',
  recoveryText: 'min-w-0 text-ib-body font-semibold text-foreground',
  recoveryButton:
    'inline-flex h-8 shrink-0 cursor-pointer items-center justify-center rounded-full border-0 bg-primary px-3 text-xs font-bold text-primary-foreground hover:bg-primary/90 disabled:cursor-wait disabled:opacity-60',
  createButton:
    'cursor-pointer !h-[42px] !rounded-full !border-0 !bg-primary !px-5 !text-sm !font-bold !tracking-normal !text-primary-foreground !shadow-none hover:!bg-primary/90 focus-visible:!ring-2 focus-visible:!ring-ring !w-full md:!w-auto',
  searchWrap:
    'ib-page-chrome-surface flex h-[42px] min-w-0 items-center gap-2 rounded-full border border-border bg-card/85 px-3 backdrop-blur-md transition-[border-color,background-color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-input focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/20',
  searchIcon: 'size-3.5 shrink-0 text-muted-foreground',
  searchInput:
    'min-w-0 flex-1 bg-transparent text-ib-body text-foreground outline-none placeholder:text-muted-foreground',
  sortSelectTrigger:
    'inline-flex h-[42px] w-[108px] min-w-[108px] cursor-pointer items-center justify-between gap-2 rounded-full border border-border bg-card/85 px-3 text-ib-body font-semibold text-foreground backdrop-blur-md shadow-xs transition-[border-color,background-color,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-input hover:bg-muted/80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring w-full md:w-[108px]',
  loadingState:
    'flex min-h-[244px] items-center justify-center rounded-xl border border-dashed border-border bg-muted/30',
  loadingSpinner: '!h-9 !w-9 text-primary',
  boardsGrid: 'grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3.5',
  createCard:
    'relative flex h-[244px] w-full flex-col cursor-pointer overflow-hidden rounded-xl border border-border bg-card p-0 text-left transition-[border-color,transform] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-border/80 hover:-translate-y-0.5 hover:scale-[1.006] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
  createPreview: `flex h-[186px] w-full items-center justify-center border-b border-border ${blankBoardPreviewSurfaceClass}`,
  createPreviewIcon:
    "relative flex size-[54px] items-center justify-center rounded-ib-panel border border-border bg-card/80 before:absolute before:top-1/2 before:left-1/2 before:h-[25px] before:w-[2.5px] before:-translate-x-1/2 before:-translate-y-1/2 before:rounded-full before:bg-muted-foreground before:content-[''] after:absolute after:top-1/2 after:left-1/2 after:h-[2.5px] after:w-[25px] after:-translate-x-1/2 after:-translate-y-1/2 after:rounded-full after:bg-muted-foreground after:content-['']",
  createMeta: 'px-3 pt-2.5 pb-3',
  createTitle: 'text-sm font-bold text-foreground',
  createDate: 'mt-[3px] text-xs text-muted-foreground',
  emptyTip: 'mt-[18px] text-ib-body text-muted-foreground',
  deleteDialogShell: 'p-5',
  deleteDialogTitle:
    'm-0 text-ib-heading leading-[1.2] font-bold tracking-normal text-foreground',
  deleteDialogDescription: 'mt-3 mb-0 text-ib-body leading-6 text-muted-foreground',
  deleteDialogFooter: 'mt-6 flex justify-end gap-2',
  deleteCancelButton:
    'inline-flex h-9 min-w-[72px] cursor-pointer items-center justify-center rounded-full border-0 bg-transparent px-3.5 text-sm font-bold text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60',
  deleteConfirmButton:
    'inline-flex h-9 min-w-[72px] cursor-pointer items-center justify-center rounded-full border-0 bg-destructive px-3.5 text-sm font-bold text-destructive-foreground transition-colors hover:bg-destructive/90 disabled:cursor-not-allowed disabled:opacity-70',
} as const
// UiDialog 以 :class/cn() 组合造型：mask 沿用 UiDialog 内置 ibDialogMaskClass。
export const catalogDeleteDialogContentClass =
  'overflow-hidden rounded-[var(--radius-board-dialog)] border border-border bg-card shadow-[var(--shadow-board-dialog)]'

export const catalogDeleteDialogStyle = {
  width: '360px',
  maxWidth: 'calc(100vw - 28px)',
} as const
