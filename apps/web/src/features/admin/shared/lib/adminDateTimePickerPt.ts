import type {
  DatePickerPassThroughMethodOptions,
  DatePickerPassThroughOptions,
} from 'primevue/datepicker'
import { ibFormInputClass } from '../../../../shared/theme/formControls'
import { adminOverlayMenuRootClass } from './adminOverlayPt'

const pickerIconButtonClass =
  'grid size-8 place-items-center rounded-[var(--radius-ib-control)] text-admin-text-2 outline-none transition-colors hover:bg-admin-surface-hover hover:text-admin-text-1 focus-visible:ring-3 focus-visible:ring-admin-focus-ring disabled:cursor-not-allowed disabled:text-admin-text-disabled'

const pickerIconButtonPt = {
  root: { class: pickerIconButtonClass },
  icon: { class: 'size-4' },
} as const

function selectableCellClass({ context }: DatePickerPassThroughMethodOptions) {
  return [
    'mx-auto grid size-9 cursor-pointer place-items-center rounded-[var(--radius-ib-control)] text-admin-body font-[650] outline-none transition-colors focus-visible:ring-3 focus-visible:ring-admin-focus-ring',
    context.selected
      ? 'bg-admin-accent text-admin-text-inverse'
      : 'text-admin-text-2 hover:bg-admin-surface-hover hover:text-admin-text-1',
    context.today && !context.selected && 'ring-1 ring-inset ring-admin-border-strong',
    context.otherMonth && !context.selected && 'text-admin-text-3',
    context.disabled && 'pointer-events-none opacity-35',
  ]
}

function selectableChoiceClass({ context }: DatePickerPassThroughMethodOptions) {
  return [
    'grid min-h-10 w-full cursor-pointer place-items-center rounded-[var(--radius-ib-control)] px-2 text-admin-body font-[700] outline-none transition-colors focus-visible:ring-3 focus-visible:ring-admin-focus-ring',
    context.selected
      ? 'bg-admin-accent text-admin-text-inverse'
      : 'text-admin-text-2 hover:bg-admin-surface-hover hover:text-admin-text-1',
    context.disabled && 'pointer-events-none opacity-35',
  ]
}

export const adminDateTimePickerActionClass =
  'rounded-[var(--radius-ib-control)] px-3 py-2 text-admin-body font-[700] text-admin-text-2 outline-none hover:bg-admin-surface-hover hover:text-admin-text-1 focus-visible:ring-3 focus-visible:ring-admin-focus-ring'

export const adminDateTimePickerPt = {
  root: { class: 'relative inline-flex w-full' },
  pcInputText: {
    root: {
      class: `${ibFormInputClass} !h-11 !bg-admin-control-bg !pr-10 !pl-10 !text-admin-body !font-[650]`,
    },
  },
  inputIconContainer: {
    class:
      'absolute inset-y-0 left-0 grid w-10 cursor-pointer place-items-center text-admin-text-3 transition-colors hover:text-admin-text-1',
  },
  inputIcon: { class: 'size-4' },
  clearIcon: {
    class:
      'absolute top-1/2 right-3 size-4 -translate-y-1/2 cursor-pointer text-admin-text-3 transition-colors hover:text-admin-text-1',
  },
  panel: {
    class: `${adminOverlayMenuRootClass} w-[min(21rem,calc(100vw-1.5rem))] p-3`,
  },
  calendarContainer: { class: 'flex' },
  calendar: { class: 'w-full' },
  header: {
    class: 'flex items-center justify-between border-b border-admin-border-soft pb-3',
  },
  pcPrevButton: pickerIconButtonPt,
  pcNextButton: pickerIconButtonPt,
  title: { class: 'flex items-center gap-1' },
  selectMonth: {
    class:
      'rounded-[var(--radius-ib-control)] px-2 py-1 text-admin-body font-[760] text-admin-text-1 outline-none hover:bg-admin-surface-hover focus-visible:ring-3 focus-visible:ring-admin-focus-ring',
  },
  selectYear: {
    class:
      'rounded-[var(--radius-ib-control)] px-2 py-1 font-mono text-admin-body font-[760] text-admin-text-2 outline-none hover:bg-admin-surface-hover focus-visible:ring-3 focus-visible:ring-admin-focus-ring',
  },
  dayView: { class: 'mt-2 w-full border-separate border-spacing-1' },
  tableHeaderCell: { class: 'p-0' },
  weekDay: {
    class: 'block py-1 text-center text-admin-caption font-[700] text-admin-text-3',
  },
  dayCell: { class: 'p-0 text-center' },
  day: selectableCellClass,
  monthView: { class: 'grid grid-cols-3 gap-2 pt-3' },
  month: selectableChoiceClass,
  yearView: { class: 'grid grid-cols-2 gap-2 pt-3' },
  year: selectableChoiceClass,
  timePicker: {
    class:
      'mt-3 flex items-center justify-center gap-2 border-t border-admin-border-soft pt-3',
  },
  hourPicker: { class: 'flex flex-col items-center gap-1' },
  minutePicker: { class: 'flex flex-col items-center gap-1' },
  separatorContainer: { class: 'flex items-center' },
  separator: { class: 'font-mono text-admin-content font-bold text-admin-text-3' },
  hour: {
    class:
      'min-w-10 rounded-[var(--radius-ib-control)] bg-admin-surface-muted px-2 py-1.5 text-center font-mono text-admin-content font-bold text-admin-text-1',
  },
  minute: {
    class:
      'min-w-10 rounded-[var(--radius-ib-control)] bg-admin-surface-muted px-2 py-1.5 text-center font-mono text-admin-content font-bold text-admin-text-1',
  },
  pcIncrementButton: pickerIconButtonPt,
  pcDecrementButton: pickerIconButtonPt,
  buttonbar: {
    class: 'mt-3 flex items-center justify-between border-t border-admin-border-soft pt-3',
  },
} satisfies DatePickerPassThroughOptions
