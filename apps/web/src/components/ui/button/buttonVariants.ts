import { cva, type VariantProps } from 'class-variance-authority'

// Single source of truth for button paint. `primary`/`danger` are aliases of
// the canonical `default`/`destructive` treatments — one definition each.
const buttonPrimaryTreatment = 'bg-primary text-primary-foreground shadow-xs hover:bg-primary-hover'
const buttonDestructiveTreatment =
  'bg-destructive text-destructive-foreground shadow-xs hover:bg-destructive-hover'

export const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-control text-sm font-medium transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0 cursor-pointer',
  {
    variants: {
      variant: {
        default: buttonPrimaryTreatment,
        primary: buttonPrimaryTreatment,
        destructive: buttonDestructiveTreatment,
        danger: buttonDestructiveTreatment,
        outline:
          'border border-input bg-background shadow-xs hover:bg-accent hover:text-accent-foreground',
        secondary: 'bg-secondary text-secondary-foreground shadow-xs hover:bg-secondary/80',
        ghost: 'hover:bg-accent hover:text-accent-foreground',
        link: 'text-primary underline-offset-4 hover:underline',
        success: 'bg-success text-success-foreground shadow-xs hover:bg-success-hover',
      },
      size: {
        default: 'h-9 px-4 py-2',
        sm: 'h-8 px-3 text-xs',
        lg: 'h-10 px-8',
        icon: 'h-9 w-9',
        action: 'h-9 px-3 text-admin-meta',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)

export type ButtonVariants = VariantProps<typeof buttonVariants>
