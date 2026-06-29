import { blankBoardPreviewSurfaceClass } from '../../canvas-board/preview'

export const boardCatalogClasses = {
  page:
    'app-scrollbar relative h-full w-full overflow-y-auto overflow-x-hidden overscroll-contain bg-[#f8f9fd] px-7 pt-4 pb-16 text-[#1f2430] [box-sizing:border-box] [-webkit-overflow-scrolling:touch] max-[860px]:px-3.5 max-[860px]:pt-3.5 max-[860px]:pb-10',
  pageBackground:
    'pointer-events-none fixed inset-0 bg-[radial-gradient(55%_35%_at_45%_45%,rgba(150,120,230,0.24)_0%,rgba(150,120,230,0)_100%),radial-gradient(circle,#e2e4e8_1px,transparent_1px)] [background-position:center_center,center_center] [background-size:100%_100%,12px_12px]',
  header: 'relative z-[1] flex items-center justify-between',
  brand: 'inline-flex min-w-0 max-w-[calc(100%-64px)] items-center',
  brandName: 'min-w-0 truncate text-[32px] font-bold tracking-normal',
  userMenuWrap: 'relative',
  userAvatarButton: 'ib-user-avatar-button',
  userAvatar: 'ib-user-avatar-button__image',
  userAvatarPlaceholder: 'ib-user-avatar-button__placeholder',
  avatarIcon: 'size-5',
  main: 'relative z-[1] mx-auto mt-[46px] max-w-[1080px]',
  pageTitle:
    'm-0 break-words text-center text-[62px] leading-[1.05] font-bold tracking-normal max-[860px]:text-[40px]',
  pageSubtitle:
    'mt-4 mb-[34px] text-center text-[19px] tracking-normal text-[#6e7487] max-[860px]:text-[15px]',
  toolbar:
    'mb-6 grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3.5 max-[860px]:grid-cols-1',
  createButton:
    '!h-[42px] !rounded-full !border-0 !bg-[#7a6bd5] !px-5 !text-sm !font-bold !tracking-normal !text-white !shadow-none hover:!bg-[#6d5fd0] focus-visible:!shadow-[0_0_0_3px_rgba(122,107,213,0.18)] max-[860px]:!w-full',
  searchWrap:
    'flex h-[42px] min-w-0 items-center gap-2 rounded-full border border-[#d9dfec] bg-white/90 px-3',
  searchIcon: 'size-3.5 shrink-0 text-[#8a90a3]',
  searchInput: 'min-w-0 flex-1 bg-transparent text-[13px] text-[#2f3446] outline-none',
  sortSelect:
    '!inline-flex !h-[42px] !w-[108px] !min-w-[108px] !items-center !justify-center !gap-2 !rounded-full !border-[#d9dfec] !bg-white/90 !text-[13px] !font-semibold !text-[#3e4457] !shadow-none max-[860px]:!w-full',
  loadingState:
    'flex min-h-[244px] items-center justify-center rounded-xl border border-dashed border-[#d9dfec] bg-white/60',
  loadingSpinner: '!h-9 !w-9 text-[#7a6bd5]',
  boardsGrid: 'grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3.5',
  createCard:
    'relative h-[244px] cursor-pointer overflow-hidden rounded-xl border border-[#d9dfec] bg-white/90 p-0 text-left transition-colors hover:border-[#cdd4e6] focus-visible:outline-none focus-visible:shadow-[0_0_0_3px_rgba(122,107,213,0.16)]',
  createPreview: `flex h-[186px] items-center justify-center border-b border-[#e7ebf3] ${blankBoardPreviewSurfaceClass}`,
  createPreviewIcon:
    "relative flex size-[54px] items-center justify-center rounded-[14px] border border-[#cfd7ea] bg-white before:absolute before:top-1/2 before:left-1/2 before:h-[25px] before:w-[2.5px] before:-translate-x-1/2 before:-translate-y-1/2 before:rounded-full before:bg-[#cfd7ea] before:content-[''] after:absolute after:top-1/2 after:left-1/2 after:h-[2.5px] after:w-[25px] after:-translate-x-1/2 after:-translate-y-1/2 after:rounded-full after:bg-[#cfd7ea] after:content-['']",
  createMeta: 'px-3 pt-2.5 pb-3',
  createTitle: 'text-sm font-bold text-[#394054]',
  createDate: 'mt-[3px] text-xs text-[#7b8297]',
  emptyTip: 'mt-[18px] text-[13px] text-[#7b8297]',
  deleteDialogShell: 'p-5',
  deleteDialogTitle:
    'm-0 text-[18px] leading-[1.2] font-bold tracking-normal text-[#2f3446]',
  deleteDialogDescription: 'mt-3 mb-0 text-[13px] leading-6 text-[#6e7487]',
  deleteDialogFooter: 'mt-6 flex justify-end gap-2',
  deleteCancelButton:
    'inline-flex h-9 min-w-[72px] cursor-pointer items-center justify-center rounded-full border-0 bg-transparent px-3.5 text-sm font-bold text-[#6e7487] transition-colors hover:bg-[#f1f4fb] disabled:cursor-not-allowed disabled:opacity-60',
  deleteConfirmButton:
    'inline-flex h-9 min-w-[72px] cursor-pointer items-center justify-center rounded-full border-0 bg-[#d14343] px-3.5 text-sm font-bold text-white transition-colors hover:bg-[#bd333a] disabled:cursor-not-allowed disabled:opacity-70',
} as const

export const boardCatalogSortSelectPt = {
  label: { class: '!w-auto !flex-none !py-0 !pr-0 !pl-0 !leading-[40px]' },
  dropdown: { class: '!w-auto !flex-none' },
  overlay: {
    class:
      '!rounded-xl !border !border-[#d8ddea] !bg-white !shadow-[0_18px_42px_rgba(45,64,89,0.14)]',
  },
  list: { class: '!p-1.5' },
  option: { class: '!rounded-lg !px-3 !py-2 !text-[13px] !font-semibold' },
} as const

export const boardCatalogDeleteDialogPt = {
  mask: { class: '!bg-[rgba(31,36,48,0.34)]' },
  root: {
    class:
      '!overflow-hidden !rounded-[20px] !border !border-[#d8ddea] !bg-white !shadow-[0_22px_58px_rgba(30,39,60,0.18)]',
  },
  content: { class: '!bg-transparent !p-0' },
} as const

export const boardCatalogDeleteDialogStyle = {
  width: '360px',
  maxWidth: 'calc(100vw - 28px)',
} as const
