import type { PrimeVueConfiguration } from 'primevue/config'

export const ideaBoardPrimePt = {
  button: {
    root: { class: '!shadow-none font-[650]' },
  },
  dialog: {
    header: { class: '!items-start !gap-3 !px-6 !pt-5 !pb-3.5' },
    headerActions: { class: '!mt-1 !shrink-0' },
    pcCloseButton: {
      root: {
        class:
          '!h-8 !w-8 !rounded-lg !border-0 !bg-transparent !p-0 !text-admin-text-3 !shadow-none hover:!bg-admin-surface-muted hover:!text-admin-text-1 focus:!shadow-none focus-visible:!outline-none focus-visible:!ring-0',
      },
      icon: { class: '!h-4 !w-4' },
    },
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
    root: { class: '!shrink-0 !whitespace-nowrap !rounded-full !font-[760]' },
  },
  textarea: {
    root: { class: '!shadow-none' },
  },
} satisfies PrimeVueConfiguration['pt']
