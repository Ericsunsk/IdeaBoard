import type { ClassValue } from 'clsx'
import type { ButtonVariants } from './buttonVariants'

// Variant names derive from the single source of truth (buttonVariants):
// `primary`/`danger` are aliases of the canonical `default`/`destructive`.
export type ButtonChromeVariant = NonNullable<ButtonVariants['variant']>

export type ButtonChromeSeverity = 'primary' | 'secondary' | 'danger' | 'success' | 'info' | 'warn'

// One definition per treatment; `primary`/`danger` alias the canonical paint.
const buttonChromePrimaryTreatment =
  'bg-primary text-primary-foreground hover:bg-primary-hover active:bg-primary-pressed shadow-xs'
const buttonChromeDestructiveTreatment =
  'bg-destructive text-destructive-foreground hover:bg-destructive-hover active:bg-destructive-pressed shadow-xs'

export const buttonChromeColorClasses: Record<ButtonChromeVariant, string> = {
  default: buttonChromePrimaryTreatment,
  primary: buttonChromePrimaryTreatment,
  secondary:
    'bg-secondary text-secondary-foreground hover:bg-secondary/80 active:bg-secondary/70 border border-border',
  destructive: buttonChromeDestructiveTreatment,
  danger: buttonChromeDestructiveTreatment,
  outline:
    'border border-input bg-card text-card-foreground hover:bg-muted active:bg-muted/80 shadow-xs',
  ghost: 'text-muted-foreground hover:bg-muted hover:text-foreground active:bg-muted/80',
  link: 'text-primary underline-offset-4 hover:underline',
  success: 'bg-success text-success-foreground hover:bg-success-hover shadow-xs',
}

export const buttonChromeBaseClass =
  'focus-visible:ring-2 focus-visible:ring-ring/20 select-none cursor-pointer disabled:cursor-not-allowed'

export function resolveButtonChromeVariant(
  variant: ButtonChromeVariant | undefined,
  options?: {
    outlined?: boolean | undefined
    severity?: ButtonChromeSeverity | undefined
    text?: boolean | undefined
  },
): ButtonChromeVariant {
  if (options?.text) {
    return 'ghost'
  }
  if (options?.outlined) {
    return 'outline'
  }
  if (options?.severity === 'danger') {
    return 'destructive'
  }
  if (options?.severity === 'secondary') {
    return 'secondary'
  }
  if (options?.severity === 'success') {
    return 'success'
  }
  return variant ?? 'default'
}

export function buttonChromeSizeTypography(size: string | undefined): ClassValue {
  if (size === 'sm') {
    return 'text-admin-meta'
  }
  if (size === 'lg') {
    return 'text-admin-title-sm'
  }
  if (!size || size === 'default') {
    return 'text-admin-body'
  }
  return false
}
