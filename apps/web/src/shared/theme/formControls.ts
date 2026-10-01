export const ibFormFieldClass =
  'flex min-w-0 flex-col gap-2 text-ib-body font-semibold text-ib-text'

export const ibFormInputIconClass =
  'pointer-events-none absolute top-1/2 left-3.5 z-10 flex -translate-y-1/2 items-center justify-center text-ib-text-subtle'

export const ibFormInputClass =
  // User-side form standard: h-10 everywhere (auth modal, account panes). The focus
  // ring matches Admin's `focus-visible:ring-2` treatment through the shared
  // `--shadow-ib-focus` token that user buttons already use — `outline-none` here
  // would otherwise leave keyboard users with only a border-color shift. Border
  // and glow both ride `focus-visible`, the same modality Admin uses, so mouse
  // and keyboard focus look identical.
  'ib-control-state [--ib-control-disabled-opacity:0.7] h-10 w-full rounded-ib-control border border-ib-border bg-ib-surface-raised px-3 text-ib-title-sm font-medium text-ib-text shadow-none outline-none transition-[border-color,background-color,color,box-shadow] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] placeholder:text-ib-text-subtle hover:border-ib-border-hover focus-visible:border-ib-creative-accent focus:bg-ib-surface focus-visible:shadow-ib-focus disabled:bg-ib-surface-disabled disabled:text-ib-text-muted'

export const ibFormInputErrorClass =
  // `!` keeps the error stroke above hover and focus borders: without it the
  // base chrome's `hover:border-ib-border-hover` (0,2,0) beats the plain error
  // border (0,1,0) and resting the mouse on an invalid field hides the error.
  // Same contract as Admin's invalid border below.
  '!border-ib-danger bg-ib-danger/5 placeholder:text-ib-danger/75 focus:bg-ib-danger/5'
