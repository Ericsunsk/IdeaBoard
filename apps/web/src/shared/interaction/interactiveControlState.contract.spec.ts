/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { appOverlayMenuItemClass } from '../theme/appOverlayMenu'
import { ibFormInputClass } from '../theme/formControls'

const controlStateStyles = readFileSync(
  resolve(process.cwd(), 'src/shared/theme/atoms/interactive-control-state.css'),
  'utf8',
)
const appStyles = readFileSync(resolve(process.cwd(), 'src/app/style.css'), 'utf8')

describe('Interactive Control State CSS contract', () => {
  it('is an app-level policy adapter driven by stable semantic attributes', () => {
    expect(appStyles).toContain("@import '../shared/theme/atoms/interactive-control-state.css';")
    expect(controlStateStyles).toContain('.ib-control-state:hover')
    expect(controlStateStyles).toContain('.ib-control-state:focus-visible')
    expect(controlStateStyles).toContain('.ib-control-state:active')
    expect(controlStateStyles).toContain("[data-control-state='loading']")
    expect(controlStateStyles).toContain("[data-control-tone='danger']")
  })

  it('uses Motion Policy and a non-layout-shifting focus outline', () => {
    expect(controlStateStyles).toContain('var(--motion-control-duration)')
    expect(controlStateStyles).toContain('var(--motion-control-easing)')
    expect(controlStateStyles).toContain('outline:')
    expect(controlStateStyles).toContain('outline-offset:')
    expect(controlStateStyles).not.toContain('border-width:')
  })

  it.each([
    ['form control', ibFormInputClass],
    ['menu item', appOverlayMenuItemClass],
  ])('keeps the %s transition in the Motion Policy utilities layer', (_name, adapter) => {
    expect(adapter).toContain('duration-[var(--motion-control-duration)]')
    expect(adapter).toContain('ease-[var(--motion-control-easing)]')
  })

  it('removes nonessential control transforms under reduced motion', () => {
    expect(controlStateStyles).toContain('@media (prefers-reduced-motion: reduce)')
    expect(controlStateStyles).toContain('transform: none;')
  })
})
