export const accountPrimaryActionButtonClass =
  'inline-flex h-11 w-full items-center justify-center rounded-[var(--radius-ib-control)] border border-transparent bg-ib-creative-accent-hover px-5 text-sm font-semibold text-ib-text-inverse shadow-[var(--shadow-ib-control)] transition-[background-color,color,box-shadow] duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] hover:bg-ib-creative-accent-pressed focus-visible:outline-none focus-visible:shadow-[var(--shadow-ib-focus)] disabled:cursor-not-allowed disabled:border-ib-border-soft disabled:bg-ib-surface-disabled disabled:text-ib-text-subtle disabled:shadow-none'

export const accountButtonPt = {
  label: { class: 'leading-none' },
  loadingIcon: { class: 'mr-2 size-4 animate-spin' },
} as const
