import type { ClassValue } from 'clsx'
import type { ButtonVariants } from './buttonVariants'

// Shared variant, severity, and typography rules for AdminButton and ActionButton.
export type ButtonChromeVariant =
  | 'default'
  | 'primary'
  | 'secondary'
  | 'destructive'
  | 'danger'
  | 'outline'
  | 'ghost'
  | 'link'
  | 'success'

export type ButtonChromeSeverity = 'primary' | 'secondary' | 'danger' | 'success' | 'info' | 'warn'

export const buttonChromeColorClasses: Record<ButtonChromeVariant, string> = {
  default:
    'bg-primary text-primary-foreground hover:bg-primary-hover active:bg-primary-pressed shadow-xs',
  primary:
    'bg-primary text-primary-foreground hover:bg-primary-hover active:bg-primary-pressed shadow-xs',
  secondary:
    'bg-secondary text-secondary-foreground hover:bg-secondary/80 active:bg-secondary/70 border border-border',
  destructive:
    'bg-destructive text-destructive-foreground hover:bg-destructive-hover active:bg-destructive-pressed shadow-xs',
  danger:
    'bg-destructive text-destructive-foreground hover:bg-destructive-hover active:bg-destructive-pressed shadow-xs',
  outline:
    'border border-input bg-card text-card-foreground hover:bg-muted active:bg-muted/80 shadow-xs',
  ghost: 'text-muted-foreground hover:bg-muted hover:text-foreground active:bg-muted/80',
  link: 'text-primary underline-offset-4 hover:underline',
  success: 'bg-emerald-700 text-white hover:bg-emerald-700/90 shadow-xs',
}

export const buttonChromeBaseClass =
  'focus-visible:ring-2 focus-visible:ring-ring/20 select-none cursor-pointer'

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
  if (size === 'small' || size === 'sm') {
    return 'text-admin-meta'
  }
  if (size === 'large' || size === 'lg') {
    return 'text-admin-title-sm'
  }
  if (!size || size === 'default') {
    return 'text-admin-body'
  }
  return false
}

export function resolveButtonChromeUiVariant(
  variant: ButtonChromeVariant,
): ButtonVariants['variant'] {
  if (variant === 'destructive' || variant === 'danger') {
    return 'destructive'
  }
  if (variant === 'outline') {
    return 'outline'
  }
  if (variant === 'secondary') {
    return 'secondary'
  }
  if (variant === 'ghost') {
    return 'ghost'
  }
  if (variant === 'link') {
    return 'link'
  }
  if (variant === 'success') {
    return 'success'
  }
  return 'default'
}
