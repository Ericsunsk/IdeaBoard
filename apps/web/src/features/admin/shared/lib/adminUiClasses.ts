export const adminSurfaceRootViewportClass =
  'flex max-w-full min-w-0 flex-col gap-4 p-0 min-[1081px]:-mb-[17px]'

export const adminSurfacePanelClass =
  'min-w-0 overflow-hidden rounded-2xl border border-admin-border-soft bg-admin-card-strong p-0'

export const adminSurfacePanelViewportClass = 'flex flex-col min-[1081px]:h-[calc(100dvh-117px)]'

export const adminSurfaceToolbarClass =
  'flex shrink-0 flex-wrap items-center justify-between gap-4 bg-transparent px-6 py-4'

export const adminSurfaceScrollContentClass =
  'app-scrollbar min-h-0 flex-1 overflow-y-auto overscroll-contain'

export const adminToolbarSearchInputClass =
  '!h-[38px] !w-full !rounded-lg !bg-white !text-[15px] !text-admin-text-1 !shadow-[0_3px_10px_rgba(45,64,89,0.05)] placeholder:!text-[#7a8798]'

export const adminToolbarMetricClass =
  'flex shrink-0 items-center gap-2 whitespace-nowrap rounded-full border border-admin-border bg-white px-3 py-1.5 text-[13px] font-bold text-admin-text-1 shadow-[0_2px_8px_rgba(45,64,89,0.02)]'

export const adminFramedIconButtonBaseClass =
  '!h-9 !w-9 !border !border-admin-border !bg-white hover:!bg-admin-surface-muted transition-colors flex items-center justify-center p-0'

export const adminFramedIconButtonClass = `${adminFramedIconButtonBaseClass} !rounded-lg`

export const adminRoundIconButtonClass = `${adminFramedIconButtonBaseClass} !rounded-full`

export const adminToolbarIconButtonClass = adminFramedIconButtonClass

export const adminToolbarPagerButtonClass = `${adminToolbarIconButtonClass} disabled:!opacity-40`

export const adminToolbarPageLabelClass =
  'flex h-9 items-center justify-center rounded-lg !border !border-admin-border !bg-white px-3 text-[12px] font-semibold text-admin-text-2'

export const adminFilterPopoverClass =
  'flex w-[260px] max-w-[calc(100vw_-_48px)] flex-col gap-4 bg-admin-card p-4'

export const adminConfigPopoverClass =
  'flex w-[280px] max-w-[calc(100vw_-_48px)] flex-col gap-4 bg-admin-card p-4'

export const adminPopoverTitleClass = 'text-admin-content font-bold text-admin-title-color'

export const adminCompactTagClass = '!px-2 !py-0.5 !text-[11px] !leading-4'
