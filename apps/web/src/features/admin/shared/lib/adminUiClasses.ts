export const adminSurfaceRootViewportClass =
  'flex max-w-full min-w-0 flex-col gap-4 p-0 min-[1081px]:-mb-[17px]'

export const adminSurfacePanelClass =
  'min-w-0 overflow-hidden rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface p-0'

export const adminSurfacePanelViewportClass = 'flex flex-col min-[1081px]:h-[calc(100dvh-117px)]'

export const adminSurfaceToolbarClass =
  'flex shrink-0 flex-wrap items-center justify-between gap-4 bg-transparent px-6 py-4'

export const adminSurfaceScrollContentClass =
  'app-scrollbar min-h-0 flex-1 overflow-y-auto overscroll-contain'

export const adminLoadingStateClass =
  'flex min-h-[260px] items-center justify-center text-admin-content text-admin-text-3'

export const adminEmptyStateClass =
  'flex min-h-[260px] flex-col items-center justify-center rounded-lg border border-ib-border-soft bg-ib-surface text-center text-ib-text-muted'

export const adminDashboardRootClass = 'flex max-w-full min-w-0 flex-col gap-4 p-0'

export const adminSummaryGridClass = 'grid grid-cols-1 gap-3.5 md:grid-cols-2 xl:grid-cols-4'

export const adminChartGridClass = 'grid grid-cols-1 gap-3.5 xl:grid-cols-12'

export const adminChartCanvasClass = 'h-full w-full min-w-0'

export const adminMasonryGridClass = 'columns-1 gap-3.5 md:columns-2 xl:columns-3'

export const adminTwoColumnMasonryGridClass = 'columns-1 gap-3.5 md:columns-2'

export const adminMasonryCardClass = 'mb-3.5 break-inside-avoid !rounded-lg'

export const adminToolbarSearchInputClass =
  '!h-[38px] !w-full !rounded-lg !bg-ib-surface !text-admin-title-sm !text-ib-text !shadow-[var(--shadow-ib-control)] placeholder:!text-ib-text-muted'

export const adminToolbarMetricClass =
  'flex shrink-0 items-center gap-2 whitespace-nowrap rounded-full border border-admin-border bg-white px-3 py-1.5 text-admin-body font-bold text-admin-text-1 shadow-[0_2px_8px_rgba(45,64,89,0.02)]'

const adminFramedIconButtonBaseClass =
  '!h-9 !w-9 !border !border-ib-border !bg-ib-surface !text-admin-text-2 hover:!bg-ib-surface-muted hover:!text-admin-text-1 disabled:!text-admin-text-3 transition-colors flex items-center justify-center p-0'

export const adminFramedIconButtonClass = `${adminFramedIconButtonBaseClass} !rounded-lg`

export const adminToolbarPagerButtonClass = `${adminFramedIconButtonClass} disabled:!opacity-40`

export const adminToolbarPageLabelClass =
  'flex h-9 items-center justify-center rounded-lg !border !border-ib-border !bg-ib-surface px-3 text-admin-meta font-semibold text-ib-text-muted'

export const adminActionButtonClass = '!h-9 !rounded-lg'

export const adminLogoButtonClass =
  'ib-control-state flex shrink-0 items-center justify-center rounded-xl border border-admin-border-soft bg-white shadow-xs cursor-pointer overflow-hidden transition-colors hover:border-admin-accent hover:bg-white focus-visible:outline-none focus-visible:border-admin-accent data-[control-state=selected]:border-admin-accent data-[control-state=selected]:bg-white'

export const adminTextButtonClass = '!text-ib-text-muted hover:!text-admin-text-1'

export const adminFilterPopoverClass =
  'flex min-w-0 w-[260px] max-w-[calc(100vw_-_48px)] flex-col gap-4 bg-ib-surface p-4'

export const adminAnomalyPopoverClass =
  'flex max-h-[calc(100dvh_-_24px)] w-[340px] max-w-[calc(100vw_-_28px)] flex-col overflow-hidden bg-ib-surface'

export const adminPopoverSectionHeaderClass = 'border-b border-admin-border pb-2'

export const adminPopoverTitleClass = 'text-admin-content font-bold text-admin-title-color'

export const adminDialogHeaderSplitClass =
  'flex w-full flex-col gap-4 pr-4 md:flex-row md:items-start md:justify-between'

export const adminDialogHeaderContentClass = 'min-w-0'

export const adminDialogTitleClass =
  'm-0 text-admin-modal-title leading-[1.15] font-bold text-admin-title-color'

export const adminDialogHeaderClass = '!px-6 !pt-5 !pb-3.5'

export const adminDialogContentClass = '!overflow-hidden !px-6 !pb-0'

export const adminDialogScrollableBodyClass =
  'app-scrollbar -mr-3 overflow-y-auto overscroll-contain pr-3 [scrollbar-gutter:stable]'

export const adminDialogFormBodyClass = 'flex flex-col gap-3.5'

export const adminDialogFooterClass = '!px-6 !pt-4 !pb-5'

export const adminDialogFooterActionsClass = 'flex w-full flex-wrap items-center justify-end gap-2'

export const adminCompactTagClass = '!px-2 !py-0.5 !text-admin-caption !leading-4'
