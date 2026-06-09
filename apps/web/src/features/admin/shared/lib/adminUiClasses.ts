export const adminSurfaceRootViewportClass =
  'flex max-w-full min-w-0 flex-col gap-4 p-0 min-[1081px]:-mb-[17px]'

export const adminSurfacePanelClass =
  'min-w-0 overflow-hidden rounded-2xl border border-admin-border-soft bg-admin-card-strong p-0'

export const adminSurfacePanelViewportClass = 'flex flex-col min-[1081px]:h-[calc(100dvh-117px)]'

export const adminSurfaceToolbarClass =
  'flex shrink-0 flex-wrap items-center justify-between gap-4 bg-transparent px-6 py-4'

export const adminSurfaceScrollContentClass =
  'app-scrollbar min-h-0 flex-1 overflow-y-auto overscroll-contain'

export const adminLoadingStateClass =
  'flex min-h-[260px] items-center justify-center text-admin-content text-admin-text-3'

export const adminEmptyStateClass =
  'flex min-h-[260px] flex-col items-center justify-center rounded-lg border border-admin-border-soft bg-admin-card text-center text-admin-text-2'

export const adminMasonryGridClass = 'columns-1 gap-3.5 md:columns-2 xl:columns-3'

export const adminMasonryCardClass = 'mb-3.5 break-inside-avoid !rounded-lg'

export const adminToolbarSearchInputClass =
  '!h-[38px] !w-full !rounded-lg !bg-white !text-admin-title-sm !text-admin-text-1 !shadow-[0_3px_10px_rgba(45,64,89,0.05)] placeholder:!text-admin-text-2'

export const adminToolbarMetricClass =
  'flex shrink-0 items-center gap-2 whitespace-nowrap rounded-full border border-admin-border bg-white px-3 py-1.5 text-admin-body font-bold text-admin-text-1 shadow-[0_2px_8px_rgba(45,64,89,0.02)]'

export const adminFramedIconButtonBaseClass =
  '!h-9 !w-9 !border !border-admin-border !bg-white hover:!bg-admin-surface-muted transition-colors flex items-center justify-center p-0'

export const adminFramedIconButtonClass = `${adminFramedIconButtonBaseClass} !rounded-lg`

export const adminRoundIconButtonClass = `${adminFramedIconButtonBaseClass} !rounded-full`

export const adminToolbarIconButtonClass = adminFramedIconButtonClass

export const adminToolbarPagerButtonClass = `${adminToolbarIconButtonClass} disabled:!opacity-40`

export const adminToolbarPageLabelClass =
  'flex h-9 items-center justify-center rounded-lg !border !border-admin-border !bg-white px-3 text-admin-meta font-semibold text-admin-text-2'

export const adminFilterPopoverClass =
  'flex w-[260px] max-w-[calc(100vw_-_48px)] flex-col gap-4 bg-admin-card p-4'

export const adminConfigPopoverClass =
  'flex w-[280px] max-w-[calc(100vw_-_48px)] flex-col gap-4 bg-admin-card p-4'

export const adminPopoverSectionHeaderClass = 'border-b border-admin-border pb-2'

export const adminPopoverTitleClass = 'text-admin-content font-bold text-admin-title-color'

export const adminDialogHeaderSplitClass =
  'flex w-full flex-col gap-4 pr-4 md:flex-row md:items-start md:justify-between'

export const adminDialogHeaderContentClass = 'min-w-0'

export const adminDialogTitleClass =
  'm-0 text-admin-modal-title leading-[1.15] font-bold text-admin-title-color'

export const adminDialogScrollableBodyClass =
  'app-scrollbar overflow-y-auto overscroll-contain pr-3'

export const adminDialogFormBodyClass = 'flex flex-col gap-3.5'

export const adminDialogFooterActionsClass = 'flex flex-wrap justify-end gap-2'

export const adminDialogCompactFooterActionsClass = 'flex justify-end gap-2.5'

export const adminCompactTagClass = '!px-2 !py-0.5 !text-admin-caption !leading-4'
