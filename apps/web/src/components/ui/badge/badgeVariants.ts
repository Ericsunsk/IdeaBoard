import { cva, type VariantProps } from 'class-variance-authority'

export const badgeVariants = cva(
  'inline-flex items-center rounded-md border px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2',
  {
    variants: {
      variant: {
        default: 'border-transparent bg-primary text-primary-foreground shadow hover:bg-primary/80',
        secondary:
          'border-transparent bg-secondary text-secondary-foreground hover:bg-secondary/80',
        destructive:
          'border-transparent bg-destructive text-destructive-foreground shadow hover:bg-destructive/80',
        outline: 'border-border text-foreground',
        success: 'border-emerald-500/20 bg-emerald-500/15 text-emerald-700 dark:text-emerald-400',
        warning: 'border-amber-500/20 bg-amber-500/15 text-amber-700 dark:text-amber-400',
        danger: 'border-destructive/20 bg-destructive/15 text-destructive',
        info: 'border-sky-500/20 bg-sky-500/15 text-sky-700 dark:text-sky-400',
      },
      rounded: {
        default: 'rounded-md',
        full: 'rounded-full',
      },
    },
    defaultVariants: {
      variant: 'default',
      rounded: 'default',
    },
  },
)

export type BadgeVariants = VariantProps<typeof badgeVariants>
