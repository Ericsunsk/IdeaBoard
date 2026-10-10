export const ibFormFieldClass =
  'flex min-w-0 flex-col gap-2 text-admin-body font-semibold text-foreground'

export const ibFormInputIconClass =
  'pointer-events-none absolute top-1/2 left-3.5 z-10 flex -translate-y-1/2 items-center justify-center text-muted-foreground'

export const ibFormInputClass =
  // User-side form standard: h-10 everywhere (auth modal, account panes).
  // Follows standard shadcn focus ring (focus-visible:ring-1 focus-visible:ring-ring),
  // with clean outline-none and no hover border mutation.
  'control-state [--control-disabled-opacity:0.7] h-10 w-full rounded-md border border-input bg-background px-3 text-admin-title-sm font-medium text-foreground shadow-xs outline-none transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] placeholder:text-muted-foreground focus-visible:ring-1 focus-visible:ring-ring disabled:bg-muted disabled:text-muted-foreground'

export const ibFormInputErrorClass =
  '!border-destructive bg-destructive/5 placeholder:text-destructive/75 focus:bg-destructive/5'
