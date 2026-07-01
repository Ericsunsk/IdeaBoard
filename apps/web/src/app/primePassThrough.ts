import type { PrimeVueConfiguration } from 'primevue/config'
import { baseDialogPt } from '../shared/theme/baseDialogPt'

const adminRouteMutedTextClass = '[body.admin-route_&]:!text-ib-text-muted'
const adminRouteDataTableRootClass =
  '[body.admin-route_&]:overflow-hidden [body.admin-route_&]:rounded-[var(--radius-ib-panel)]'
const adminRouteDataTableHeaderCellClass =
  '[body.admin-route_&]:!border-ib-border-soft [body.admin-route_&]:!bg-ib-surface-muted [body.admin-route_&]:!text-admin-meta [body.admin-route_&]:!font-[720] [body.admin-route_&]:!tracking-normal [body.admin-route_&]:!normal-case [body.admin-route_&]:!text-ib-text-muted'
const adminRouteDataTableBodyCellClass =
  '[body.admin-route_&]:!border-ib-border-soft [body.admin-route_&]:!bg-white/70 [body.admin-route_&]:!text-ib-text'

function classValueContains(value: unknown, needle: string): boolean {
  if (!value) return false
  if (typeof value === 'string') return value.includes(needle)
  if (Array.isArray(value)) return value.some((item) => classValueContains(item, needle))
  if (typeof value === 'object') {
    return Object.keys(value).some((key) => key.includes(needle))
  }
  return false
}

function isTextButton(props: { text?: boolean; variant?: string }) {
  return props.text || props.variant === 'text'
}

export const ideaBoardPrimePt = {
  button: {
    root: ({ props }) => ({
      class: [
        '!shadow-none font-[650]',
        isTextButton(props) && !classValueContains(props.class, 'active')
          ? `${adminRouteMutedTextClass} hover:[body.admin-route_&]:!text-admin-text-1`
          : '',
      ],
    }),
  },
  dialog: baseDialogPt,
  datatable: {
    root: { class: adminRouteDataTableRootClass },
    column: {
      headerCell: {
        class: adminRouteDataTableHeaderCellClass,
      },
      bodyCell: {
        class: adminRouteDataTableBodyCellClass,
      },
    },
  },
  inputnumber: {
    pcInputText: {
      root: { class: '!shadow-none' },
    },
  },
  inputtext: {
    root: { class: '!shadow-none' },
  },
  inputicon: {
    root: { class: adminRouteMutedTextClass },
  },
  autocomplete: {
    root: {
      class: '!shadow-none',
    },
    inputMultiple: { class: '!shadow-none' },
  },
  paginator: {
    root: { class: '!gap-1 !border-0 !bg-transparent !p-0' },
  },
  select: {
    root: { class: '!shadow-none' },
  },
  tag: {
    root: { class: '!shrink-0 !whitespace-nowrap !rounded-full !font-[760]' },
  },
  textarea: {
    root: { class: '!shadow-none' },
  },
} satisfies PrimeVueConfiguration['pt']
