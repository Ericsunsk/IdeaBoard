import type { DataTablePassThroughOptions } from 'primevue/datatable'

const adminDataTableRootClass = 'overflow-hidden !rounded-none'
const adminDataTableHeaderCellClass =
  '!border-ib-border-soft !bg-ib-surface-muted !px-4 !py-2.5 !text-left !text-admin-meta !font-[720] !tracking-normal !normal-case !text-ib-text-muted'
const adminDataTableBodyCellClass =
  '!border-ib-border-soft !bg-white/70 !px-4 !py-2.5 !text-left !align-middle !text-ib-text'

export const adminDenseDataTablePt = {
  root: { class: adminDataTableRootClass },
  bodyRow: { class: 'last:[&>td]:!border-b-0' },
  column: {
    headerCell: { class: adminDataTableHeaderCellClass },
    bodyCell: { class: adminDataTableBodyCellClass },
  },
} satisfies DataTablePassThroughOptions
