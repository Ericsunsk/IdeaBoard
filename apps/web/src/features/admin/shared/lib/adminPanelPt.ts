import type { PanelPassThroughOptions } from 'primevue/panel'

type AdminPanelDensity = 'default' | 'compact'

function createAdminPanelPt(density: AdminPanelDensity = 'default') {
  const compact = density === 'compact'
  return {
    root: {
      class: '!overflow-hidden !rounded-lg !border-ib-border-soft !bg-ib-surface !shadow-none',
    },
    header: {
      class: [
        '!border-ib-border-soft !bg-transparent !text-ib-text',
        compact ? '!px-3 !py-2' : '!px-4 !py-3',
      ].join(' '),
    },
    content: {
      class: [
        '!border-ib-border-soft !bg-white/70',
        compact ? '!px-3 !py-2.5' : '!px-4 !py-3',
      ].join(' '),
    },
  } satisfies PanelPassThroughOptions
}

export const adminPanelPt = createAdminPanelPt()
