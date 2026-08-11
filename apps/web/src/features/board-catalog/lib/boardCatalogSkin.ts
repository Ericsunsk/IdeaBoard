export const blankBoardPreviewSurfaceClass =
  'bg-[radial-gradient(circle,#dfe4ef_1px,transparent_1px),linear-gradient(#fbfcff,#fbfcff)] [background-size:12px_12px,cover]'
import { pageChromeAccountMenuClasses } from '../../../shared/lib/pageChrome'
import { ibDialogMaskPt } from '../../../shared/theme/dialogMask'

export const boardCatalogClasses = {
  page: 'app-scrollbar relative h-full w-full overflow-y-auto overflow-x-hidden overscroll-contain bg-board-page px-[var(--spacing-ib-page-chrome-inset)] pt-[var(--spacing-ib-page-chrome-inset)] pb-16 text-board-page-text [box-sizing:border-box] [-webkit-overflow-scrolling:touch] max-[860px]:pb-10',
  pageBackground: 'ib-board-page-background ib-page-dot-bg',
  header: 'relative z-[1] flex items-start justify-between',
  brand: 'inline-flex min-w-0 max-w-[calc(100%-64px)] items-center',
  userMenuWrap: pageChromeAccountMenuClasses.wrap,
  userAvatarButton: pageChromeAccountMenuClasses.avatarButton,
  userAvatar: pageChromeAccountMenuClasses.avatarImage,
  userAvatarPlaceholder: pageChromeAccountMenuClasses.avatarPlaceholder,
  avatarIcon: pageChromeAccountMenuClasses.avatarIcon,
  main: 'relative z-[1] mx-auto mt-[46px] max-w-[1080px]',
  pageTitle:
    'm-0 break-words text-center text-[62px] leading-[1.05] font-bold tracking-normal max-[860px]:text-[40px]',
  pageSubtitle:
    'mt-4 mb-[34px] text-center text-[19px] tracking-normal text-board-page-text-muted max-[860px]:text-[15px]',
  toolbar:
    'mb-6 grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3.5 max-[860px]:grid-cols-1',
  recoveryState:
    'mb-4 flex min-h-10 items-center justify-between gap-3 rounded-[var(--radius-ib-panel)] border border-board-card-border bg-board-control-bg px-3 py-2',
  recoveryText: 'min-w-0 text-[13px] font-semibold text-board-control-text',
  recoveryButton:
    'inline-flex h-8 shrink-0 cursor-pointer items-center justify-center rounded-full border-0 bg-board-primary px-3 text-xs font-bold text-ib-text-inverse hover:bg-board-primary-hover disabled:cursor-wait disabled:opacity-60',
  createButton:
    '!h-[42px] !rounded-full !border-0 !bg-board-primary !px-5 !text-sm !font-bold !tracking-normal !text-ib-text-inverse !shadow-none hover:!bg-board-primary-hover focus-visible:!shadow-[var(--shadow-board-focus-strong)] max-[860px]:!w-full',
  searchWrap:
    'flex h-[42px] min-w-0 items-center gap-2 rounded-full border border-board-card-border bg-board-control-bg px-3',
  searchIcon: 'size-3.5 shrink-0 text-board-control-icon',
  searchInput:
    'min-w-0 flex-1 bg-transparent text-[13px] text-board-control-input-text outline-none',
  sortSelect:
    '!inline-flex !h-[42px] !w-[108px] !min-w-[108px] !items-center !justify-center !gap-2 !rounded-full !border-board-card-border !bg-board-control-bg !text-[13px] !font-semibold !text-board-control-text !shadow-none max-[860px]:!w-full',
  loadingState:
    'flex min-h-[244px] items-center justify-center rounded-xl border border-dashed border-board-card-border bg-board-loading-bg',
  loadingSpinner: '!h-9 !w-9 text-board-primary',
  boardsGrid: 'grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3.5',
  createCard:
    'relative h-[244px] cursor-pointer overflow-hidden rounded-xl border border-board-card-border bg-board-card-bg p-0 text-left transition-colors hover:border-board-card-border-hover focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)]',
  createPreview: `flex h-[186px] items-center justify-center border-b border-board-card-preview-divider ${blankBoardPreviewSurfaceClass}`,
  createPreviewIcon:
    "relative flex size-[54px] items-center justify-center rounded-[var(--radius-ib-panel)] border border-board-create-icon bg-board-surface-strong before:absolute before:top-1/2 before:left-1/2 before:h-[25px] before:w-[2.5px] before:-translate-x-1/2 before:-translate-y-1/2 before:rounded-full before:bg-board-create-icon before:content-[''] after:absolute after:top-1/2 after:left-1/2 after:h-[2.5px] after:w-[25px] after:-translate-x-1/2 after:-translate-y-1/2 after:rounded-full after:bg-board-create-icon after:content-['']",
  createMeta: 'px-3 pt-2.5 pb-3',
  createTitle: 'text-sm font-bold text-board-create-title',
  createDate: 'mt-[3px] text-xs text-board-card-text',
  emptyTip: 'mt-[18px] text-[13px] text-board-card-text',
  deleteDialogShell: 'p-5',
  deleteDialogTitle:
    'm-0 text-[18px] leading-[1.2] font-bold tracking-normal text-board-dialog-title',
  deleteDialogDescription: 'mt-3 mb-0 text-[13px] leading-6 text-board-dialog-description',
  deleteDialogFooter: 'mt-6 flex justify-end gap-2',
  deleteCancelButton:
    'inline-flex h-9 min-w-[72px] cursor-pointer items-center justify-center rounded-full border-0 bg-transparent px-3.5 text-sm font-bold text-board-dialog-description transition-colors hover:bg-ib-surface-hover disabled:cursor-not-allowed disabled:opacity-60',
  deleteConfirmButton:
    'inline-flex h-9 min-w-[72px] cursor-pointer items-center justify-center rounded-full border-0 bg-board-danger px-3.5 text-sm font-bold text-ib-text-inverse transition-colors hover:bg-board-danger-hover disabled:cursor-not-allowed disabled:opacity-70',
} as const

export const boardCatalogSortSelectPt = {
  label: { class: '!w-auto !flex-none !py-0 !pr-0 !pl-0 !leading-[40px]' },
  dropdown: { class: '!w-auto !flex-none' },
  overlay: {
    class:
      '!rounded-xl !border !border-board-menu-border !bg-board-surface-strong !shadow-[var(--shadow-ib-overlay)]',
  },
  list: { class: '!p-1.5' },
  option: { class: '!rounded-lg !px-3 !py-2 !text-[13px] !font-semibold' },
} as const

export const boardCatalogDeleteDialogPt = {
  ...ibDialogMaskPt,
  root: {
    class:
      '!overflow-hidden !rounded-[var(--radius-board-dialog)] !border !border-board-menu-border !bg-board-surface-strong !shadow-[var(--shadow-board-dialog)]',
  },
  content: { class: '!bg-transparent !p-0' },
} as const

export const boardCatalogDeleteDialogStyle = {
  width: '360px',
  maxWidth: 'calc(100vw - 28px)',
} as const
