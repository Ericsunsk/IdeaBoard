export const ibFormFieldClass = 'flex min-w-0 flex-col gap-2 text-[13px] font-semibold text-ib-text'

export const ibFormInputIconClass =
  'pointer-events-none absolute top-1/2 left-3.5 z-10 flex -translate-y-1/2 items-center justify-center text-ib-text-subtle'

export const ibFormInputClass =
  'ib-control-state [--ib-control-disabled-opacity:0.7] h-12 w-full rounded-[var(--radius-ib-control)] border border-ib-border bg-ib-surface-raised px-3 text-[15px] font-medium text-ib-text shadow-none outline-none transition-[border-color,background-color,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] placeholder:text-ib-text-subtle hover:border-ib-border-hover focus:border-ib-creative-accent focus:bg-ib-surface disabled:bg-ib-surface-disabled disabled:text-ib-text-muted'

export const ibFormInputErrorClass =
  'border-ib-danger bg-ib-danger/5 placeholder:text-ib-danger/75 focus:border-ib-danger focus:bg-ib-danger/5'

export const ibFormErrorMessagePt = {
  root: {
    class: 'm-0 mt-1.5 border-0 bg-transparent p-0 text-[12px] font-semibold text-ib-danger',
  },
  contentWrapper: { class: 'contents' },
  content: { class: 'contents' },
  text: { class: 'contents' },
} as const

const passwordToggleIconClass =
  'absolute top-1/2 right-3.5 size-5 -translate-y-1/2 cursor-pointer text-ib-text-subtle transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:text-ib-text'

export function createIbPasswordPt(inputClass = 'pr-12') {
  return {
    root: { class: 'relative block w-full' },
    pcInputText: {
      root: {
        class: inputClass,
      },
    },
    maskIcon: {
      class: passwordToggleIconClass,
    },
    unmaskIcon: {
      class: passwordToggleIconClass,
    },
  } as const
}

export const ibPasswordPt = createIbPasswordPt()
