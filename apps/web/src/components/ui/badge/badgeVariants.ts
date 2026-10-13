import { cva, type VariantProps } from 'class-variance-authority'

export const badgeVariants = cva(
  'inline-flex items-center border px-2.5 py-0.5 text-xs font-semibold transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
  {
    variants: {
      variant: {
        default:
          'border-transparent bg-primary text-primary-foreground shadow hover:bg-primary-hover',
        secondary:
          'border-transparent bg-secondary text-secondary-foreground hover:bg-secondary/80',
        destructive:
          'border-transparent bg-destructive text-destructive-foreground shadow hover:bg-destructive-hover',
        outline: 'border-border text-foreground',
        success: 'border-success/20 bg-success-soft text-success',
        warning: 'border-warning/20 bg-warning-soft text-warning',
        danger: 'border-destructive/20 bg-destructive/15 text-destructive',
        info: 'border-info/20 bg-info-soft text-info',
      },
      rounded: {
        default: 'rounded-control',
        full: 'rounded-pill',
      },
    },
    defaultVariants: {
      variant: 'default',
      rounded: 'default',
    },
  },
)

export type BadgeVariants = VariantProps<typeof badgeVariants>
