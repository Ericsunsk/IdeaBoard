import type { DataTablePassThroughOptions } from 'primevue/datatable'

export const adminDenseDataTablePt = {
  column: {
    headerCell: { class: '!py-2.5' },
    bodyCell: { class: '!py-2.5' },
  },
} satisfies DataTablePassThroughOptions
