export const adminSurfaceRootViewportClass =
  'flex max-w-full min-w-0 flex-col gap-4 p-0'

export const adminSurfacePanelClass =
  'min-w-0 overflow-hidden rounded-ib-panel border border-ib-border-soft bg-ib-surface p-0'

export const adminSurfacePanelViewportClass = 'flex flex-col min-[1081px]:h-[calc(100dvh-192px)]'

export const adminSurfaceToolbarClass =
  'flex shrink-0 flex-wrap items-center justify-between gap-4 bg-transparent px-6 py-4'

export const adminSurfaceScrollContentClass =
  'app-scrollbar min-h-0 flex-1 overflow-y-auto overscroll-contain'

export const adminEmptyStateClass =
  'flex min-h-[260px] flex-col items-center justify-center rounded-lg border border-ib-border-soft bg-ib-surface text-center text-ib-text-muted'

export const adminDashboardRootClass = 'flex max-w-full min-w-0 flex-col gap-4 p-0'

// 数据表壳单源：任务监控事件表与定价表同壳（容器 + AdminTable 属性 + thead/tbody token），
// 改表样式只改这里，两表一起变。
export const adminTableCardClass =
  'flex w-full flex-col overflow-hidden rounded-xl border border-admin-border-soft bg-admin-bg'

export const adminTableFrameClass = 'min-w-0'

export const adminTableStyle = { minWidth: '100%' } as const

export const adminTableClass =
  'border-separate border-spacing-0 [&_thead]:bg-admin-bg [&_th]:h-10 [&_th]:text-xs [&_th]:font-semibold [&_th]:text-admin-text-2 [&_th]:border-b-0 [&_tbody]:bg-white [&_tbody_tr:first-child_td]:border-t [&_tbody_tr:first-child_td]:border-admin-border-soft [&_tbody_tr:first-child_td:first-child]:rounded-tl-xl [&_tbody_tr:first-child_td:last-child]:rounded-tr-xl [&_tbody_tr:last-child_td]:border-b-0'

export const adminTableFooterClass =
  'flex items-center justify-between border-t border-admin-border-soft bg-admin-bg px-4 py-2.5'

export const adminTableRowActionButtonClass =
  'flex size-8 shrink-0 cursor-pointer items-center justify-center rounded-lg border border-admin-border-soft bg-white text-admin-text-2 transition-colors hover:bg-admin-surface-muted'

export const adminSummaryGridClass = 'grid grid-cols-1 gap-3.5 md:grid-cols-2 xl:grid-cols-4'

export const adminChartGridClass = 'grid grid-cols-1 gap-3.5 xl:grid-cols-12'

export const adminChartCanvasClass = 'h-full w-full min-w-0'



export const adminToolbarMetricClass =
  'flex shrink-0 items-center gap-2 whitespace-nowrap rounded-full border border-admin-border bg-white px-3 py-1.5 text-admin-body font-bold text-admin-text-1 shadow-[0_2px_8px_rgba(45,64,89,0.02)]'

const adminFramedIconButtonBaseClass =
  '!h-9 !w-9 !border !border-ib-border !bg-ib-surface !text-admin-text-2 hover:!bg-ib-surface-muted hover:!text-admin-text-1 disabled:!text-admin-text-3 transition-colors flex items-center justify-center p-0'

export const adminFramedIconButtonClass = `${adminFramedIconButtonBaseClass} !rounded-lg`

/**
 * 紧凑型独立控制按钮 (Compact Control Button)
 * 供 Events 折叠按钮、表格底栏翻页按钮等共享同一套视觉 Token
 */
export const adminCompactIconButtonClass =
  'flex !size-7 shrink-0 items-center justify-center !rounded-lg !border !border-admin-border-soft !bg-white !p-0 !text-admin-text-2 transition-colors hover:!bg-admin-surface-muted hover:!text-admin-text-1 cursor-pointer disabled:!cursor-not-allowed disabled:!opacity-40'

export const adminToolbarPageLabelClass =
  'flex h-7 items-center justify-center rounded-lg border border-admin-border-soft bg-white px-2.5 text-xs font-medium text-admin-text-2'

export const adminActionButtonClass = '!h-9 !rounded-lg'

export const adminLogoButtonClass =
  'ib-control-state flex shrink-0 items-center justify-center rounded-xl border border-admin-border-soft bg-white shadow-xs cursor-pointer overflow-hidden transition-colors hover:border-admin-accent hover:bg-white focus-visible:outline-none focus-visible:border-admin-accent data-[control-state=selected]:border-admin-accent data-[control-state=selected]:bg-white'

export const adminTextButtonClass = '!text-ib-text-muted hover:!text-admin-text-1'

export const adminAnomalyPopoverClass =
  'flex max-h-[calc(100dvh_-_24px)] w-[340px] max-w-[calc(100vw_-_28px)] flex-col overflow-hidden bg-ib-surface'

export const adminDialogHeaderSplitClass =
  'flex w-full flex-col gap-4 pr-4 md:flex-row md:items-start md:justify-between'

export const adminDialogHeaderContentClass = 'min-w-0'

export const adminDialogTitleClass =
  'm-0 text-admin-modal-title leading-[1.15] font-bold text-admin-title-color'

export const adminDialogHeaderClass = '!px-6 !pt-5 !pb-3.5'

export const adminDialogScrollableBodyClass =
  'app-scrollbar -mr-3 overflow-y-auto overscroll-contain pr-3 [scrollbar-gutter:stable]'

export const adminDialogFormBodyClass = 'flex flex-col gap-3.5'

export const adminDialogFooterActionsClass = 'flex w-full flex-wrap items-center justify-end gap-2'

// 全局唯一 Admin 弹窗 chrome（原 models 页 bordered 语言）：容器圆角与表面均出自
// admin token，不再引用 ib 别名；各弹窗不再自带 chrome 分支。
export const adminDialogChromeClass =
  'rounded-[var(--radius-admin-dialog)] border-admin-border-soft bg-admin-card-strong'

export const adminDialogChromeHeaderClass =
  '!py-4.5 border-b border-admin-border-soft bg-admin-card-strong'

export const adminDialogChromeHeaderSplitClass =
  '!flex-row !items-center !justify-between !gap-2 !pr-0'

export const adminDialogChromeTitleClass =
  '!text-admin-title !font-bold text-admin-title-color tracking-tight'

export const adminDialogChromeContentClass = '!overflow-hidden !px-6 !py-6 bg-admin-card-strong'

export const adminDialogChromeFooterClass =
  '!px-6 !py-4 border-t border-admin-border-soft bg-admin-card-strong'

export const adminDialogChromeCloseClass = 'text-admin-text-3 hover:text-admin-text-1'

export const adminCompactTagClass = '!px-2 !py-0.5 !text-admin-caption !leading-4'

// 全局唯一输入框 chrome（AdminInput/AdminNumberInput/AdminTextarea 共用）：
// 无阴影扁平底，hover 只加深边框，focus 用柔光 ring；尺寸/字号由各组件按 size 追加覆盖。
export const adminInputChromeClass =
  'flex w-full rounded-lg border border-admin-border bg-white text-admin-text-1 transition-colors placeholder:text-admin-text-3 hover:border-admin-border-hover focus-visible:border-admin-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-admin-accent/20 disabled:cursor-not-allowed disabled:opacity-50'
