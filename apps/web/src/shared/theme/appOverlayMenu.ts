export const appOverlayMenuThemeClass =
  '[--app-overlay-menu-item-color:var(--color-ib-overlay-item)] [--app-overlay-menu-item-hover-bg:var(--color-ib-overlay-item-hover-bg)] [--app-overlay-menu-item-hover-color:var(--color-ib-overlay-item)] [--app-overlay-menu-danger-color:var(--color-ib-overlay-danger)] [--app-overlay-menu-danger-hover-bg:var(--color-ib-overlay-danger-hover-bg)] [--app-overlay-menu-danger-hover-color:var(--color-ib-overlay-danger)]'

export const appOverlayMenuSurfaceClass = `!border !border-[var(--color-ib-overlay-border)] !bg-[var(--color-ib-overlay-bg)] ${appOverlayMenuThemeClass}`

export const appOverlayMenuItemClass =
  'ib-control-state flex w-full items-center rounded-lg px-2.5 py-2 text-[13px] font-semibold text-[var(--app-overlay-menu-item-color,var(--color-ib-text-muted))] no-underline transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-[var(--app-overlay-menu-item-hover-bg,var(--color-ib-surface-hover))] hover:text-[var(--app-overlay-menu-item-hover-color,var(--color-ib-text))] focus-visible:shadow-ib-focus'

export const appOverlayMenuDangerItemClass =
  '!text-[var(--app-overlay-menu-danger-color,var(--color-ib-danger))] hover:!bg-[var(--app-overlay-menu-danger-hover-bg,var(--color-ib-surface-hover))] hover:!text-[var(--app-overlay-menu-danger-hover-color,var(--color-ib-danger))]'
