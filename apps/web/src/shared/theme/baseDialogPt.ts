import type { DialogPassThroughOptions } from 'primevue/dialog'

export const baseDialogPt = {
  header: { class: '!items-start !gap-3 !px-6 !pt-5 !pb-3.5' },
  headerActions: { class: '!mt-1 !shrink-0' },
  pcCloseButton: {
    root: {
      class:
        '!h-8 !w-8 !rounded-lg !border-0 !bg-transparent !p-0 !text-admin-text-3 !shadow-none hover:!bg-admin-surface-muted hover:!text-admin-text-1 focus:!shadow-none focus-visible:!outline-none focus-visible:!ring-0',
    },
    icon: { class: '!h-4 !w-4' },
  },
} satisfies DialogPassThroughOptions
