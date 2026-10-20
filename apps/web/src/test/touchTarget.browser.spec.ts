import '@testing-library/jest-dom/vitest'

// Touch-target floor browser contract.
//
// Observable proof for the touch-target floor: asserts rendered geometry
// (offset boxes and `::after` hit areas) and live token linkage, never file
// text. `scripts/checkTouchTargetContract.ts` stays the presence guard for
// every registered control; this spec proves each mechanism it guards has
// the specified rendered effect.
//
// Placement: `src/test/` hosts cross-cutting browser contracts (see
// `p1ThemeContracts.browser.spec.ts`). The spec lives here rather than next
// to `src/theme/atoms/touch-target.css` because the `horizontal-not-to-features`
// dependency-cruiser rule forbids `src/theme/**` from importing concrete
// feature modules, and this contract renders real registered controls.
//
// Coverage map against the checker's control list:
// - `--spacing-touch-target-min` 32px floor + live linkage: `declares ...`
// - `--spacing-touch-target-aa` 24px drag exception: `declares ...`
// - `.hit-target-32` both axes / `.hit-target-32-inline` block axis / click
//   still routes to the real element: `grows visually-small controls ...`
//   (mechanism shared by the admin tab trigger, chart range segment, task
//   filter pill, node action button, and the three canvas chrome buttons).
// - AdminSwitch track (20px visual, opt-in overlay): `keeps the AdminSwitch ...`
// - NodeActionButton (`min-h-7` visual, inline overlay): `keeps node action ...`
// - Compact admin icon button (remediated real `!size-8` box): `keeps the compact ...`
// - Curve drag point (`hit-target-drag`, keyboard-operable): `stops retouch drag ...`
// - Histogram marker (transparent box grown to the AA floor, painted dot
//   untouched): `stops retouch drag ...`

import { cleanup } from '@testing-library/vue'
import { page } from 'vitest/browser'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { renderWithUser, screen, waitFor } from './render'
import { i18n } from '../core/i18n'
import AdminSwitch from '../features/admin/shared/ui/form/AdminSwitch.vue'
import { NodeActionButton } from '../features/canvas/ui/nodes/primitives'
import { adminCompactIconButtonClass } from '../features/admin/shared/core/uiClasses'
import '../app/style.css'
import {
  CurveGraph,
  normalizeCurveChannelPoints,
} from '../features/canvas/tools/retouch/testing/browser'

const FLOOR_PX = 32
const AA_PX = 24

function rootToken(name: string): string {
  return window.getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

function afterBox(element: Element): { width: number; height: number } {
  const style = window.getComputedStyle(element, '::after')
  return { width: Number.parseFloat(style.width), height: Number.parseFloat(style.height) }
}

function probeHost(tag: string, className: string, cssText: string): HTMLElement {
  const host = document.createElement(tag)
  host.setAttribute('data-touch-probe', '')
  host.className = className
  host.style.cssText = cssText
  document.body.append(host)
  return host
}

describe('touch-target floor browser contract', () => {
  beforeEach(async () => {
    await page.viewport(1280, 800)
    document.documentElement.classList.add('light')
    i18n.global.locale.value = 'en'
  })

  afterEach(() => {
    cleanup()
    for (const host of document.querySelectorAll('[data-touch-probe]')) host.remove()
    document.documentElement.style.removeProperty('--spacing-touch-target-min')
    document.documentElement.style.removeProperty('--spacing-touch-target-aa')
  })

  it('declares the 32px floor and the 24px drag exception as live tokens', async () => {
    expect(rootToken('--spacing-touch-target-min')).toBe(`${FLOOR_PX}px`)
    expect(rootToken('--spacing-touch-target-aa')).toBe(`${AA_PX}px`)

    // Provenance: the hit area tracks the token live, so a hardcoded 32px
    // overlay cannot satisfy this.
    const host = probeHost('button', 'hit-target-32', 'width: 20px; height: 20px;')
    expect(afterBox(host).width).toBe(FLOOR_PX)
    document.documentElement.style.setProperty('--spacing-touch-target-min', '40px')
    try {
      await expect.poll(() => afterBox(host).width).toBe(40)
    } finally {
      document.documentElement.style.removeProperty('--spacing-touch-target-min')
    }
    await expect.poll(() => afterBox(host).width).toBe(FLOOR_PX)
  })

  it('grows visually-small controls to the floor without changing the design', () => {
    const small = probeHost('button', 'hit-target-32', 'width: 20px; height: 20px;')
    expect(small.offsetWidth).toBe(20)
    expect(small.offsetHeight).toBe(20)
    expect(afterBox(small)).toEqual({ width: FLOOR_PX, height: FLOOR_PX })

    const chip = probeHost('button', 'hit-target-32-inline', 'width: 80px; height: 20px;')
    expect(afterBox(chip).height).toBe(FLOOR_PX)
    expect(afterBox(chip).width).toBe(80)

    // The pseudo-element belongs to the control itself: activation still
    // routes to the real element.
    const clicked = vi.fn()
    small.addEventListener('click', clicked)
    small.click()
    expect(clicked).toHaveBeenCalledOnce()
  })

  it('keeps the AdminSwitch track on the floor without inflating the design', async () => {
    const rendered = renderWithUser(AdminSwitch)
    const track = screen.getByRole('switch')
    expect(track.className).toContain('hit-target-32')
    expect(track.offsetHeight).toBe(20)
    expect(afterBox(track).height).toBe(FLOOR_PX)

    track.click()
    await waitFor(() => expect(rendered.emitted()['update:modelValue']).toBeTruthy())
  })

  it('keeps node action buttons on the floor inside compact chrome', () => {
    renderWithUser(NodeActionButton, { props: { ariaLabel: 'Add node' } })
    const action = screen.getByRole('button', { name: 'Add node' })
    expect(action.className).toContain('hit-target-32-inline')
    expect(action.offsetHeight).toBeLessThan(FLOOR_PX)
    expect(afterBox(action).height).toBe(FLOOR_PX)
  })

  it('keeps the compact admin icon button on a real 32px box', () => {
    const icon = probeHost('button', adminCompactIconButtonClass, '')
    expect(icon.offsetWidth).toBe(FLOOR_PX)
    expect(icon.offsetHeight).toBe(FLOOR_PX)
  })

  it('stops retouch drag handles at the 24px exception and keeps them keyboard-operable', () => {
    const points = normalizeCurveChannelPoints([{ id: 'mid', input: 50, output: 50 }])
    const events = {
      addPoint: vi.fn(),
      movePoint: vi.fn(),
      removePoint: vi.fn(),
      selectPoint: vi.fn(),
    }
    renderWithUser(
      defineComponent({
        setup: () => () =>
          h(CurveGraph, {
            activeCurvePath: 'M 0 100 L 100 0',
            ghostCurvePaths: [],
            histogramPath: '',
            points,
            selectedPointId: 'mid',
            onAddPoint: events.addPoint,
            onMovePoint: events.movePoint,
            onRemovePoint: events.removePoint,
            onSelectPoint: events.selectPoint,
          }),
      }),
    )
    const mid = screen.getByRole('button', { name: 'Curve point In 50 Out 50' })
    expect(mid.className).toContain('hit-target-drag')
    // The exception: a full 32px overlay would steal neighbouring grabs, so
    // the handle stops at the WCAG AA floor instead of the repo floor.
    expect(afterBox(mid)).toEqual({ width: AA_PX, height: AA_PX })

    mid.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true }))
    expect(events.movePoint).toHaveBeenCalledWith('mid', { input: 50, output: 51 })
    mid.dispatchEvent(new KeyboardEvent('keydown', { key: 'Delete', bubbles: true }))
    expect(events.removePoint).toHaveBeenCalledWith('mid')

    // The histogram marker grows its transparent box to the same AA floor
    // while the painted dot keeps its visual size (both pseudos already
    // paint the marker, so no overlay class is involved).
    const marker = probeHost('button', 'retouch-histogram-plot__marker', '')
    expect(marker.offsetWidth).toBe(AA_PX)
    expect(Number.parseFloat(window.getComputedStyle(marker, '::after').width)).toBe(12)
  })
})
