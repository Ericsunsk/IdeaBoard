// Shared overlay-menu chrome (account menus, canvas board menu, library island).
// The theme class pins each [--app-overlay-menu-*] prop to its defined overlay
// token so subtree overrides stay optional; item reads fall back to the same
// defined values, never to lookalike Tier 2 roles. Surface and state styling
// uses standard color utilities (bg-overlay-bg, border-overlay-border,
// text-overlay-item, …) with no `!` escapes: cn() merge order lets the caller
// class win over primitive bases.
export const appOverlayMenuThemeClass =
  '[--app-overlay-menu-item-color:var(--color-overlay-item)] [--app-overlay-menu-item-hover-bg:var(--color-overlay-item-hover-bg)] [--app-overlay-menu-item-hover-color:var(--color-overlay-item)] [--app-overlay-menu-danger-color:var(--color-overlay-danger)] [--app-overlay-menu-danger-hover-bg:var(--color-overlay-danger-hover-bg)] [--app-overlay-menu-danger-hover-color:var(--color-overlay-danger)]'

export const appOverlayMenuSurfaceClass = `border border-overlay-border bg-overlay-bg ${appOverlayMenuThemeClass}`

export const appOverlayMenuItemClass =
  'control-state flex w-full items-center rounded-control px-2.5 py-2 text-admin-body font-semibold text-[var(--app-overlay-menu-item-color,var(--color-overlay-item))] no-underline transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-[var(--app-overlay-menu-item-hover-bg,var(--color-overlay-item-hover-bg))] hover:text-[var(--app-overlay-menu-item-hover-color,var(--color-overlay-item))] focus-visible:shadow-focus-ring'

export const appOverlayMenuDangerItemClass =
  'text-[var(--app-overlay-menu-danger-color,var(--color-overlay-danger))] hover:bg-[var(--app-overlay-menu-danger-hover-bg,var(--color-overlay-danger-hover-bg))] hover:text-[var(--app-overlay-menu-danger-hover-color,var(--color-overlay-danger))]'

// Keep portal menus visually separated from their trigger across page chrome surfaces.
export const appOverlayMenuSideOffset = 10
