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
    expect(appOverlayMenuSurfaceClass).toContain('var(--color-overlay-border)')
    expect(appOverlayMenuSurfaceClass).toContain('var(--color-overlay-bg)')
    expect(appOverlayMenuThemeClass).toContain('var(--color-overlay-item)')
    expect(appOverlayMenuThemeClass).toContain('var(--color-overlay-danger)')
    expect(appOverlayMenuSurfaceClass).toContain(appOverlayMenuThemeClass)
  })

  it('provides accessible menu item and danger item classes', () => {
    expect(appOverlayMenuItemClass).toContain('control-state')
    expect(appOverlayMenuItemClass).toContain('rounded-control')
    expect(appOverlayMenuDangerItemClass).toContain('var(--app-overlay-menu-danger-color')
  })
})
