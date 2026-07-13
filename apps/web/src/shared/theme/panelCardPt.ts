import type { CardPassThroughOptions } from 'primevue/card'

export const panelCardPt = {
  root: {
    class:
      '!overflow-hidden !rounded-[var(--radius-ib-panel)] !border !border-ib-border-soft !bg-ib-surface !shadow-[var(--shadow-ib-panel)]',
  },
  body: { class: '!px-6 !py-5' },
} satisfies CardPassThroughOptions
