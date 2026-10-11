/**
 * Admin select skin — single source of truth.
 *
 * Every Admin dropdown (form select, option filter, native `<select>`) reads its
 * geometry, border, focus ring, and overlay motion from here. They used to
 * disagree: `rounded-lg` vs `rounded-xl`, `h-9` vs `h-8`, `admin-border` vs
 * `admin-border-soft`, and two popover-based dropdowns carried no focus ring at
 * all — the same WCAG 2.4.7 gap that `PasswordInput` had.
 *
 * Reka `Select` supplies the keyboard model (arrow keys, typeahead, Home/End,
 * type-to-change) that a hand-rolled `Popover` + `role="listbox"` cannot, so
 * every option list must build on `SelectRoot`, not `PopoverRoot`.
 */

/** The value surface, shared by `Select` and native `<select>`. */
export const adminSelectTriggerClass =
  'h-9 w-full min-w-0 cursor-pointer rounded-lg border border-input bg-background px-3 text-admin-body text-foreground transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/20 disabled:cursor-not-allowed disabled:opacity-50'

export const adminSelectTriggerCompactClass = 'h-8 px-2.5 text-admin-meta'

/** Active/filtered state for a dropdown trigger, used by filter-style selects. */
export const adminSelectTriggerActiveClass = 'border-primary bg-primary-faint-soft text-primary'

export const adminSelectTriggerIconClass = 'size-4 shrink-0 text-muted-foreground opacity-70'

/** The option list overlay. One motion contract, one radius, one offset. */
export const adminSelectContentClass =
  'relative z-admin-dialog overflow-hidden rounded-lg border border-border bg-popover text-foreground outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2'

/** Every dropdown overlay detaches from its trigger by the same distance. */
export const ADMIN_SELECT_SIDE_OFFSET = 4

export const adminSelectViewportClass = 'w-full min-w-[var(--reka-select-trigger-width)] p-1.5'

export const adminSelectItemClass =
  'relative flex w-full cursor-pointer select-none items-center rounded-lg py-1.5 pr-8 pl-2 text-admin-body text-foreground outline-none transition-colors focus:bg-muted focus:text-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50'

export const adminSelectItemCompactClass = 'py-1 pl-2 pr-7 text-admin-meta'

export const adminSelectItemIndicatorClass =
  'absolute right-2 flex size-3.5 items-center justify-center'

export const adminSelectItemCheckClass = 'size-4 text-primary'

/** Overlay panels that host more than a plain option list. */
export const adminSelectPanelClass =
  'z-admin-dialog w-[260px] max-w-[calc(100vw_-_48px)] rounded-lg border border-border bg-card p-4 text-foreground outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=top]:slide-in-from-bottom-2'
