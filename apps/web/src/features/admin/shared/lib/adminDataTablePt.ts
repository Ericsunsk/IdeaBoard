import type { DataTablePassThroughOptions } from 'primevue/datatable'

export const adminDenseDataTablePt = {
  root: { class: '!rounded-none' },
  bodyRow: { class: 'last:[&>td]:!border-b-0' },
  column: {
    headerCell: { class: '!bg-ib-surface !px-4 !py-2.5 !text-left' },
    bodyCell: { class: '!px-4 !py-2.5 !text-left !align-middle' },
  },
} satisfies DataTablePassThroughOptions
