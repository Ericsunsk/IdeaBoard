export const appOverlayMenuThemeClass =
  '[--app-overlay-menu-item-color:var(--color-overlay-item)] [--app-overlay-menu-item-hover-bg:var(--color-overlay-item-hover-bg)] [--app-overlay-menu-item-hover-color:var(--color-overlay-item)] [--app-overlay-menu-danger-color:var(--color-overlay-danger)] [--app-overlay-menu-danger-hover-bg:var(--color-overlay-danger-hover-bg)] [--app-overlay-menu-danger-hover-color:var(--color-overlay-danger)]'

export const appOverlayMenuSurfaceClass = `!border !border-[var(--color-overlay-border)] !bg-[var(--color-overlay-bg)] ${appOverlayMenuThemeClass}`

export const appOverlayMenuItemClass =
  'control-state flex w-full items-center rounded-lg px-2.5 py-2 text-admin-body font-semibold text-[var(--app-overlay-menu-item-color,var(--muted-foreground))] no-underline transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-[var(--app-overlay-menu-item-hover-bg,var(--accent-hover))] hover:text-[var(--app-overlay-menu-item-hover-color,var(--foreground))] focus-visible:shadow-[0_0_0_3px_var(--ring)]'

export const appOverlayMenuDangerItemClass =
  '!text-[var(--app-overlay-menu-danger-color,var(--destructive))] hover:!bg-[var(--app-overlay-menu-danger-hover-bg,var(--muted))] hover:!text-[var(--app-overlay-menu-danger-hover-color,var(--destructive))]'

// Keep portal menus visually separated from their trigger across page chrome surfaces.
export const appOverlayMenuSideOffset = 10
