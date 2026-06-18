import type { PrimeVueConfiguration } from 'primevue/config'
import { baseDialogPt } from '../shared/theme/baseDialogPt'

const adminRouteMutedTextClass = '[body.admin-route_&]:!text-[#718096]'

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
    root: { class: 'overflow-hidden rounded-2xl' },
    column: {
      headerCell: {
        class:
          '!border-admin-border-soft !bg-admin-surface-muted !text-admin-meta !font-[720] !tracking-normal !normal-case !text-admin-text-2',
      },
      bodyCell: {
        class: '!border-admin-border-soft !bg-white/70 !text-admin-text-1',
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
