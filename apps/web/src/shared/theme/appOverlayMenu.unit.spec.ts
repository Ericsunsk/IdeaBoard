/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  appOverlayMenuThemeClass,
  appOverlayMenuSurfaceClass,
  appOverlayMenuItemClass,
  appOverlayMenuDangerItemClass,
} from './appOverlayMenu'

const appStyles = readFileSync(resolve(process.cwd(), 'src/app/style.css'), 'utf8')

describe('App overlay menu contract', () => {
  it('owns cross-feature surface and menu state tokens', () => {
    for (const token of [
      '--color-overlay-bg',
      '--color-overlay-border',
      '--color-overlay-item',
      '--color-overlay-item-hover-bg',
      '--color-overlay-danger',
      '--color-overlay-danger-hover-bg',
      '--shadow-overlay-menu',
    ]) {
      expect(appStyles).toContain(token)
    }

    expect(appOverlayMenuSurfaceClass).toContain('var(--color-overlay-border)')
    expect(appOverlayMenuSurfaceClass).toContain('var(--color-overlay-bg)')
    expect(appOverlayMenuThemeClass).toContain('var(--color-overlay-item)')
    expect(appOverlayMenuThemeClass).toContain('var(--color-overlay-danger)')
    expect(appOverlayMenuSurfaceClass).toContain(appOverlayMenuThemeClass)
  })

  it('provides accessible menu item and danger item classes', () => {
    expect(appOverlayMenuItemClass).toContain('control-state')
    expect(appOverlayMenuItemClass).toContain('rounded-lg')
    expect(appOverlayMenuDangerItemClass).toContain('var(--app-overlay-menu-danger-color')
  })
})
