import type { CardPassThroughOptions } from 'primevue/card'

export const adminCardPt = {
  root: {
    class:
      '!overflow-hidden !rounded-2xl !border !border-admin-border-soft !bg-admin-card-strong !shadow-[0_4px_20px_-2px_rgba(45,64,89,0.02),0_2px_6px_-1px_rgba(45,64,89,0.01)]',
  },
  body: { class: '!px-6 !py-5' },
} satisfies CardPassThroughOptions
