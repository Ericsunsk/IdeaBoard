/**
 * Neutral Table skin, shadcn-vue standard.
 *
 * Semantics live in the native elements the components below render
 * (`table`/`thead`/`tbody`/`tfoot`/`tr`/`th`/`td`/`caption`); this file owns
 * only the neutral geometry, which every surface then overrides through
 * explicit class props. It must not carry surface-specific values.
 *
 * All skins are plain strings, not `cva()`: a zero-variant `cva()` returns
 * a *function*, and `cn()`/`clsx()` silently drops functions — the base
 * classes would never reach the DOM.
 */
export const tableHeaderClasses = '[&_tr]:border-b'

export const tableBodyClasses = '[&_tr:last-child]:border-0'

export const tableFooterClasses = 'border-t bg-muted/50 font-medium [&>tr]:last:border-b-0'

export const tableRowClasses =
  'border-b transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-muted/50 data-[state=selected]:bg-muted'

export const tableHeadClasses =
  'h-10 px-2 text-left align-middle font-medium text-muted-foreground [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]'

export const tableCellClasses =
  'p-2 align-middle [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]'

/**
 * Sortable header button. `min-h-8` holds the 32px touch-target floor from
 * `theme/touch-target.css` so dense tables do not need a per-instance
 * opt-in; the header row itself stays at `h-10`.
 */
export const tableSortButtonClasses =
  'inline-flex min-h-8 items-center gap-1 rounded-control transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1'
