import type { PrimeVueConfiguration } from 'primevue/config'

export const ideaBoardPrimePt = {
  button: {
    root: { class: '!shadow-none font-[650]' },
  },
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
    bodyRow: { class: 'hover:[&>td]:!bg-admin-surface-hover' },
  },
  inputnumber: {
    pcInputText: {
      root: { class: '!shadow-none' },
    },
  },
  inputtext: {
    root: { class: '!shadow-none' },
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
    root: { class: '!rounded-full !font-[760]' },
  },
  textarea: {
    root: { class: '!shadow-none' },
  },
} satisfies PrimeVueConfiguration['pt']
