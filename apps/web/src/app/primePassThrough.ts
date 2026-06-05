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
      root: { class: '!shadow-none focus:!shadow-[0_0_0_3px_var(--color-admin-focus-ring)]' },
    },
  },
  inputtext: {
    root: { class: '!shadow-none focus:!shadow-[0_0_0_3px_var(--color-admin-focus-ring)]' },
  },
  autocomplete: {
    root: {
      class:
        '[&.p-focus_.p-autocomplete-input-multiple]:!shadow-[0_0_0_3px_var(--color-admin-focus-ring)]',
    },
    inputMultiple: { class: '!shadow-none' },
  },
  paginator: {
    root: { class: '!gap-1 !border-0 !bg-transparent !p-0' },
  },
  select: {
    root: { class: '!shadow-none [&.p-focus]:!shadow-[0_0_0_3px_var(--color-admin-focus-ring)]' },
  },
  tag: {
    root: { class: '!rounded-full !font-[760]' },
  },
  textarea: {
    root: { class: '!shadow-none focus:!shadow-[0_0_0_3px_var(--color-admin-focus-ring)]' },
  },
} satisfies PrimeVueConfiguration['pt']
