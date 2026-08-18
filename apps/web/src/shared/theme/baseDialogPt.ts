import type { DialogPassThroughOptions } from 'primevue/dialog'
import { ibDialogMaskPt } from './dialogMask'

export const baseDialogPt = {
  ...ibDialogMaskPt,
  header: { class: '!items-start !gap-3 !px-6 !pt-5 !pb-3.5' },
  headerActions: { class: '!mt-1 !shrink-0' },
  pcCloseButton: {
    root: {
      class:
        '!h-8 !w-8 !rounded-lg !border-0 !bg-transparent !p-0 !text-ib-text-subtle !shadow-none hover:!bg-transparent hover:!text-ib-text focus:!outline-none focus:!shadow-none focus-visible:!outline-none focus-visible:!shadow-none focus-visible:!ring-0',
    },
    icon: { class: '!h-4 !w-4' },
  },
} satisfies DialogPassThroughOptions
