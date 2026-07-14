import type { PrimeVueConfiguration } from 'primevue/config'
import { baseDialogPt } from '../shared/theme/baseDialogPt'

export const ideaBoardPrimePt = {
  button: {
    root: { class: '!shadow-none font-[650]' },
  },
  dialog: baseDialogPt,
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
  datatable: {
    column: {
      headerCell: {
        class: 'text-admin-body',
        style: { verticalAlign: 'middle' },
      },
      bodyCell: {
        style: { verticalAlign: 'middle' },
      },
    },
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
