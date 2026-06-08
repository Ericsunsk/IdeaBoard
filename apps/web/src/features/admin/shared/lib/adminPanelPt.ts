import type { PanelPassThroughOptions } from 'primevue/panel'

type AdminPanelDensity = 'default' | 'compact'

export function createAdminPanelPt(density: AdminPanelDensity = 'default') {
  const compact = density === 'compact'
  return {
    root: {
      class: '!overflow-hidden !rounded-lg !border-admin-border-soft !bg-admin-card !shadow-none',
    },
    header: {
      class: [
        '!border-admin-border-soft !bg-transparent !text-admin-text-1',
        compact ? '!px-3 !py-2' : '!px-4 !py-3',
      ].join(' '),
    },
    content: {
      class: [
        '!border-admin-border-soft !bg-white/70',
        compact ? '!px-3 !py-2.5' : '!px-4 !py-3',
      ].join(' '),
    },
  } satisfies PanelPassThroughOptions
}

export const adminPanelPt = createAdminPanelPt()

export const adminCompactPanelPt = createAdminPanelPt('compact')
