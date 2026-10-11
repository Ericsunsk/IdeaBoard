// Admin 单元格原子 class 单源：AdminMono / 行内跳转 / 截断都从这里取。
// - adminMonoClass：数字 / 金额 / 日期 / 单号的 font-mono 收口（AdminMono.vue 使用）。
// - adminNumericCellClass：数值 / 日期列追加（ledger/orders 范本：tabular-nums + whitespace-nowrap）。
// - adminInlineJumpClass：行内跳转 button 范本（以 orders 表单号 button 为准）。
// - adminTruncateCellClass：截断规范 truncate + title；调用方必须同时给 :title，
//   finding 的 line-clamp-2 是保留特例，不走这里。
export const adminMonoClass = 'font-mono'

export const adminNumericCellClass = 'tabular-nums whitespace-nowrap'

export const adminInlineJumpClass =
  'cursor-pointer rounded font-mono text-xs font-semibold tabular-nums whitespace-nowrap text-foreground transition-colors hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/20'

export const adminTruncateCellClass = 'truncate'
