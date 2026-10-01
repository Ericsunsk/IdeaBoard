import { cva, type VariantProps } from 'class-variance-authority'

export const inputVariants = cva(
  // No focus border here by contract: every surface skin owns its own focus
  // treatment (Admin: accent border + ring-2; user: accent border + shadow
  // ring). The old `focus-visible:border-ring` tied at (0,2,0) with the skins'
  // solid accent border and, depending on stylesheet order, won — leaving a
  // faint glow with no visible stroke.
  'flex w-full rounded-md border border-input bg-transparent transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50',
  {
    variants: {
      size: {
        default: 'h-9 px-3 py-1 text-base md:text-sm',
        sm: 'h-8 px-2.5 py-1 text-xs',
        compact: 'h-8 px-2.5 py-1 text-xs',
      },
    },
    defaultVariants: {
      size: 'default',
    },
  },
)

export type InputVariants = VariantProps<typeof inputVariants>
