import '@testing-library/jest-dom/vitest'

/**
 * Touch-target hit-bounds browser contract: the hit-area overlays grow
 * the pointer target past the visual box without changing layout, pointer
 * activation routes to the real control, and the fine/coarse/precision
 * profile tokens hold their pinned values.
 */

import { afterEach, describe, expect, it } from 'vitest'
import '../app/style.css'

function probe(): HTMLDivElement {
  const root = document.createElement('div')
  root.style.cssText = 'position:absolute;left:0;top:0;'
  document.body.append(root)
  return root
}

function tokensOf(element: Element): Record<string, string> {
  const style = window.getComputedStyle(element)
  return {
    min: style.getPropertyValue('--spacing-touch-target-min').trim(),
    precision: style.getPropertyValue('--spacing-touch-target-aa').trim(),
    coarse: style.getPropertyValue('--spacing-touch-target-coarse').trim(),
  }
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('touch-target profile tokens', () => {
  it('pins the fine, precision, and coarse profiles', () => {
    const root = probe()
    expect(tokensOf(root)).toEqual({ min: '32px', precision: '24px', coarse: '44px' })
  })
})

describe('touch-target hit bounds', () => {
  it('grows the hit area past a small visual without changing layout', () => {
    const root = probe()
    const plain = document.createElement('button')
    plain.textContent = 'plain'
    plain.style.cssText = 'width:20px;height:20px;padding:0;border:0;'
    const grown = document.createElement('button')
    grown.textContent = 'grown'
    grown.className = 'hit-target-32'
    grown.style.cssText = 'width:20px;height:20px;padding:0;border:0;'
    root.append(plain, grown)

    // The overlay must not move layout: identical boxes with/without it.
    expect(grown.offsetWidth).toBe(plain.offsetWidth)
    expect(grown.offsetHeight).toBe(plain.offsetHeight)

    const overlay = window.getComputedStyle(grown, '::after')
    expect(Number.parseFloat(overlay.width)).toBeGreaterThanOrEqual(32)
    expect(Number.parseFloat(overlay.height)).toBeGreaterThanOrEqual(32)
  })

  it('routes activation at the overhang point to the real control', () => {
    const root = probe()
    let activated = 0
    const control = document.createElement('button')
    control.textContent = 'target'
    control.className = 'hit-target-32'
    control.style.cssText =
      'width:20px;height:20px;padding:0;border:0;margin:40px;position:relative;'
    control.addEventListener('click', () => {
      activated += 1
    })
    root.append(control)

    const rect = control.getBoundingClientRect()
    // 3px past the visual edge: inside the 32px overlay, outside the 20px box.
    const target = document.elementFromPoint(rect.left + 23, rect.top + 10)
    expect(target).toBe(control)
    ;(target as HTMLElement | null)?.click()
    expect(activated).toBe(1)
  })

  it('keeps the drag precision floor at the AA minimum', () => {
    const root = probe()
    const handle = document.createElement('button')
    handle.className = 'hit-target-drag'
    handle.style.cssText = 'width:11px;height:11px;padding:0;border:0;'
    root.append(handle)
    const overlay = window.getComputedStyle(handle, '::after')
    expect(Number.parseFloat(overlay.width)).toBeGreaterThanOrEqual(24)
    expect(Number.parseFloat(overlay.height)).toBeGreaterThanOrEqual(24)
  })
})
