/**
 * Neutral Tabs skin, shadcn-vue standard.
 *
 * Structure and keyboard behaviour live in reka `Tabs` (roving tabindex,
 * arrow/Home/End navigation, disabled handling); this file owns only the
 * neutral geometry, which every surface then overrides through explicit
 * class props. It must not carry surface-specific values: Admin and the
 * user shell both mount this primitive and own their own skin.
 *
 * Variant-less skins are plain strings, not `cva()`: a zero-variant `cva()`
 * returns a *function*, and `cn()`/`clsx()` silently drops functions — the
 * base classes would never reach the DOM.
 */
export const tabsListVariants =
  'inline-flex h-9 items-center justify-center rounded-control bg-muted p-1 text-muted-foreground'

export const tabsTriggerVariants =
  'inline-flex min-h-8 items-center justify-center whitespace-nowrap rounded-control px-3 py-1 text-sm font-medium ring-offset-background transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 data-[state=active]:bg-background data-[state=active]:text-foreground data-[state=active]:shadow'

export const tabsContentVariants =
  'mt-2 ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2'
