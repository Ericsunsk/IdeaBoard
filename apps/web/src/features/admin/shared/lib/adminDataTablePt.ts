import type { DataTablePassThroughOptions } from 'primevue/datatable'

export const adminDenseDataTablePt = {
  root: { class: '!rounded-none' },
  bodyRow: { class: 'last:[&>td]:!border-b-0' },
  column: {
    headerCell: { class: '!bg-white !py-2.5' },
    bodyCell: { class: '!py-2.5' },
  },
} satisfies DataTablePassThroughOptions
