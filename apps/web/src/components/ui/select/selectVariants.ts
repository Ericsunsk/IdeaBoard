import { type VariantProps, cva } from 'class-variance-authority'

/**
 * Neutral Select skin, shadcn-vue standard.
 *
 * Structure and keyboard behaviour live in reka `Select`; this file owns only
 * the shadcn default geometry, which every surface then overrides through
 * explicit class props. It must not carry surface-specific values: Admin and the
 * user shell both mount this primitive and own their own skin.
 *
 * Consumers needing a different level pass class props, exactly as
 * `Popover`/`DropdownMenu` callers do. Do not add an Admin or Board token here.
 */
export const selectTriggerVariants = cva(
  'flex w-full items-center justify-between gap-2 rounded-control border border-input bg-transparent px-3 py-2 text-sm whitespace-nowrap shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 data-[placeholder]:text-muted-foreground [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0 [&_svg]:text-muted-foreground',
  {
    variants: {
      size: {
        default: 'h-9',
        sm: 'h-8 px-2.5 text-xs',
      },
    },
    defaultVariants: {
      size: 'default',
    },
  },
)

/**
 * Variant-less skins are plain strings, not `cva()`: a zero-variant `cva()`
 * returns a *function*, and `cn()`/`clsx()` silently drops functions — the
 * base classes would never reach the DOM. Only the trigger, which actually
 * varies by size, uses `cva()`.
 */
export const selectContentVariants =
  'relative z-50 max-h-96 min-w-32 overflow-hidden rounded-panel border border-border bg-popover text-popover-foreground shadow-md outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2'

export const selectItemVariants =
  'relative flex w-full cursor-default select-none items-center gap-2 rounded-control py-1.5 pr-8 pl-2 text-sm outline-none transition-colors focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50'

export const selectViewportVariants = 'w-full min-w-[var(--reka-select-trigger-width)] p-1'

export type SelectTriggerVariants = VariantProps<typeof selectTriggerVariants>
