import { cva, type VariantProps } from 'class-variance-authority'

export const inputVariants = cva(
  'flex w-full rounded-md border border-input bg-transparent shadow-sm transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50',
  {
    variants: {
      size: {
        default: 'h-9 px-3 py-1 text-base md:text-sm',
        sm: 'h-8 px-2.5 py-1 text-xs',
        compact: 'h-8 px-2.5 py-1 text-xs',
      },
      invalid: {
        true: 'border-destructive focus-visible:ring-destructive',
        false: '',
      },
    },
    defaultVariants: {
      size: 'default',
      invalid: false,
    },
  },
)

export type InputVariants = VariantProps<typeof inputVariants>
