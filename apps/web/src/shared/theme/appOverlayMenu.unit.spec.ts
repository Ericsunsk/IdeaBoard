/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  appOverlayMenuThemeClass,
  appOverlayMenuSurfaceClass,
  createAppOverlayMenuPt,
} from './appOverlayMenu'

const appStyles = readFileSync(resolve(process.cwd(), 'src/app/style.css'), 'utf8')

describe('App overlay menu contract', () => {
  it('owns cross-feature surface and menu state tokens', () => {
    for (const token of [
      '--color-ib-overlay-bg',
      '--color-ib-overlay-border',
      '--color-ib-overlay-item',
      '--color-ib-overlay-item-hover-bg',
      '--color-ib-overlay-danger',
      '--color-ib-overlay-danger-hover-bg',
      '--shadow-ib-overlay-menu',
    ]) {
      expect(appStyles).toContain(token)
    }

    expect(appOverlayMenuSurfaceClass).toContain('var(--color-ib-overlay-border)')
    expect(appOverlayMenuSurfaceClass).toContain('var(--color-ib-overlay-bg)')
    expect(appOverlayMenuThemeClass).toContain('var(--color-ib-overlay-item)')
    expect(appOverlayMenuThemeClass).toContain('var(--color-ib-overlay-danger)')
    expect(appOverlayMenuSurfaceClass).toContain(appOverlayMenuThemeClass)
    expect(createAppOverlayMenuPt('test').root).toEqual({
      class: expect.stringContaining('var(--shadow-ib-overlay-menu)'),
    })
  })

  it('builds PrimeVue menu structure while leaving geometry caller-owned', () => {
    const pt = createAppOverlayMenuPt('test-menu !w-40')

    expect(pt.root).toEqual({
      class: expect.stringContaining('test-menu !w-40'),
    })
    expect(pt.list).toEqual({ class: expect.stringContaining('!flex !flex-col') })
    expect(pt.itemContent).toEqual({ class: '!rounded-lg' })
  })
})
