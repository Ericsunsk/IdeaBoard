export const ibFormFieldClass =
  'flex min-w-0 flex-col gap-2 text-admin-body font-semibold text-foreground'

export const ibFormInputIconClass =
  'pointer-events-none absolute top-1/2 left-3.5 z-10 flex -translate-y-1/2 items-center justify-center text-muted-foreground'

export const ibFormInputClass =
  // User-side form standard: h-10 everywhere (auth modal, account panes). The focus
  // ring matches Admin's `focus-visible:ring-2` treatment through the shared
  // `ring-ring` token that user buttons already use — `outline-none` here
  // would otherwise leave keyboard users with only a border-color shift. Border
  // and glow both ride `focus-visible`, the same modality Admin uses, so mouse
  // and keyboard focus look identical.
  'control-state [--control-disabled-opacity:0.7] h-10 w-full rounded-control border border-input bg-background px-3 text-admin-title-sm font-medium text-foreground shadow-none outline-none transition-[border-color,background-color,color,box-shadow] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] placeholder:text-muted-foreground hover:border-border-hover focus-visible:border-primary focus:bg-background focus-visible:ring-2 focus-visible:ring-ring/20 disabled:bg-muted disabled:text-muted-foreground'

export const ibFormInputErrorClass =
  // `!` keeps the error stroke above hover and focus borders: without it the
  // base chrome's `hover:border-border-hover` (0,2,0) beats the plain error
  // border (0,1,0) and resting the mouse on an invalid field hides the error.
  // Same contract as Admin's invalid border below.
  '!border-destructive bg-destructive/5 placeholder:text-destructive/75 focus:bg-destructive/5'
