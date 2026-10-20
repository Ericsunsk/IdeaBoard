/// <reference types="node" />

import { describe, expect, it } from 'vitest'
import {
  appOverlayMenuThemeClass,
  appOverlayMenuSurfaceClass,
  appOverlayMenuItemClass,
  appOverlayMenuDangerItemClass,
} from './appOverlayMenu'

describe('App overlay menu contract', () => {
  it('owns cross-feature surface and menu state tokens', () => {
    expect(appOverlayMenuSurfaceClass).toContain('border-overlay-border')
    expect(appOverlayMenuSurfaceClass).toContain('bg-overlay-bg')
    expect(appOverlayMenuSurfaceClass).not.toContain('!')
    expect(appOverlayMenuThemeClass).toContain('var(--color-overlay-item)')
    expect(appOverlayMenuThemeClass).toContain('var(--color-overlay-danger)')
    expect(appOverlayMenuSurfaceClass).toContain(appOverlayMenuThemeClass)
  })

  it('falls back to the defined overlay values, never lookalike roles', () => {
    expect(appOverlayMenuItemClass).toContain(
      'text-[var(--app-overlay-menu-item-color,var(--color-overlay-item))]',
    )
    expect(appOverlayMenuItemClass).toContain(
      'hover:bg-[var(--app-overlay-menu-item-hover-bg,var(--color-overlay-item-hover-bg))]',
    )
    expect(appOverlayMenuItemClass).toContain(
      'hover:text-[var(--app-overlay-menu-item-hover-color,var(--color-overlay-item))]',
    )
    expect(appOverlayMenuDangerItemClass).toContain(
      'hover:bg-[var(--app-overlay-menu-danger-hover-bg,var(--color-overlay-danger-hover-bg))]',
    )
    for (const cls of [appOverlayMenuItemClass, appOverlayMenuDangerItemClass]) {
      expect(cls).not.toContain('var(--muted-foreground)')
      expect(cls).not.toContain('var(--accent-hover)')
      expect(cls).not.toContain('var(--foreground)')
      expect(cls).not.toContain('var(--muted)')
      expect(cls).not.toContain('!')
    }
  })

  it('provides accessible menu item and danger item classes', () => {
    expect(appOverlayMenuItemClass).toContain('control-state')
    expect(appOverlayMenuItemClass).toContain('rounded-control')
    expect(appOverlayMenuDangerItemClass).toContain('var(--app-overlay-menu-danger-color')
  })
})
