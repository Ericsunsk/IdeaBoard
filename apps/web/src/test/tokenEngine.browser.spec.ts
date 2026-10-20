import '@testing-library/jest-dom/vitest'

// Theme utilities resolve live runtime roles on the root and in subtrees.

import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import '../app/style.css'

function probe(): HTMLDivElement {
  const root = document.createElement('div')
  root.style.cssText = 'position:absolute;left:-9999px;top:0;'
  document.body.append(root)
  return root
}

function paintOf(root: HTMLElement, property: string, value: string): string {
  const el = document.createElement('div')
  el.style.setProperty('width', '10px')
  el.style.setProperty('height', '10px')
  el.style.setProperty(property, value)
  root.append(el)
  return window.getComputedStyle(el).getPropertyValue(property)
}

function utilityPaint(root: HTMLElement, className: string, property: string): string {
  const el = document.createElement('div')
  el.className = className
  root.append(el)
  return window.getComputedStyle(el).getPropertyValue(property)
}

afterEach(() => {
  document.documentElement.classList.remove('light')
  document.body.innerHTML = ''
})

describe('token engine paint equivalence (light)', () => {
  beforeEach(() => document.documentElement.classList.add('light'))

  it.each([
    ['--background', 'background-color', 'rgb(251, 251, 251)'],
    ['--foreground', 'color', 'rgb(23, 23, 23)'],
    ['--primary', 'background-color', 'rgb(109, 95, 208)'],
    ['--card', 'background-color', 'rgb(255, 255, 255)'],
    ['--border', 'border-color', 'rgb(228, 228, 231)'],
    ['--ring', 'color', 'rgba(122, 107, 213, 0.25)'],
    ['--destructive', 'color', 'rgb(220, 38, 38)'],
    // Resting tool accent keeps white action text above 4.5:1.
    ['--color-tool-accent', 'background-color', 'rgb(114, 95, 208)'],
    ['--color-canvas-workspace-bg', 'background-color', 'rgb(231, 231, 231)'],
    ['--color-admin-bg', 'background-color', 'rgb(251, 251, 251)'],
  ] as const)('%s paints %s', (token, property, expected) => {
    const root = probe()
    expect(paintOf(root, property, `var(${token})`)).toBe(expected)
  })

  it('emits bare utilities through the bridge', () => {
    const root = probe()
    expect(utilityPaint(root, 'bg-primary', 'background-color')).toBe('rgb(109, 95, 208)')
    expect(utilityPaint(root, 'text-primary-foreground', 'color')).toBe('rgb(255, 255, 255)')
    expect(utilityPaint(root, 'border-border', 'border-color')).toBe('rgb(228, 228, 231)')
  })
})

describe('token engine default dark and subtree resolution', () => {
  it('resolves dark roles and native controls without a theme class', () => {
    expect(document.documentElement.classList.contains('light')).toBe(false)
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(window.getComputedStyle(document.documentElement).colorScheme).toBe('dark')
    const root = probe()
    expect(paintOf(root, 'background-color', 'var(--background)')).toBe('rgb(13, 13, 13)')
    expect(paintOf(root, 'background-color', 'var(--primary)')).toBe('rgb(252, 252, 252)')
    expect(paintOf(root, 'background-color', 'var(--color-canvas-workspace-bg)')).toBe(
      'rgb(15, 15, 15)',
    )
  })

  it('resolves light roles and utilities in a light subtree of the default dark root', () => {
    const root = probe()
    root.classList.add('light')
    expect(paintOf(root, 'background-color', 'var(--background)')).toBe('rgb(251, 251, 251)')
    expect(utilityPaint(root, 'bg-primary', 'background-color')).toBe('rgb(109, 95, 208)')
    expect(paintOf(root, 'background-color', 'var(--color-canvas-node-bg)')).toBe(
      'rgb(247, 247, 247)',
    )
  })

  it.each(['root', 'subtree'])('keeps elevation utilities live in a light %s', (scope) => {
    const root = probe()
    expect(utilityPaint(root, 'shadow-board-dialog', 'box-shadow')).toContain(
      'rgba(0, 0, 0, 0.65) 0px 22px 58px 0px',
    )
    expect(utilityPaint(root, 'shadow-auth-primary', 'box-shadow')).toBe('none')
    expect(utilityPaint(root, 'shadow-auth-primary-hover', 'box-shadow')).toBe('none')

    const lightScope = scope === 'root' ? document.documentElement : root
    lightScope.classList.add('light')

    expect(utilityPaint(root, 'shadow-board-dialog', 'box-shadow')).toContain(
      'rgba(0, 0, 0, 0.12) 0px 22px 58px 0px',
    )
    for (const [utility, role] of [
      ['shadow-auth-primary', '--auth-primary-shadow'],
      ['shadow-auth-primary-hover', '--auth-primary-shadow-hover'],
    ] as const) {
      const expectedShadow = paintOf(root, 'box-shadow', `var(${role})`)
      expect(expectedShadow).not.toBe('none')
      expect(utilityPaint(root, utility, 'box-shadow')).toContain(expectedShadow)
    }
  })

  it('follows element-level runtime overrides through utilities', () => {
    const root = probe()
    const el = document.createElement('div')
    el.className = 'bg-primary'
    el.style.setProperty('--primary', '#ff0000')
    root.append(el)
    expect(window.getComputedStyle(el).backgroundColor).toBe('rgb(255, 0, 0)')
  })
})

describe('domain token runtime contracts', () => {
  beforeEach(() => document.documentElement.classList.add('light'))

  it('resolves admin and board roles to their canonical paint', () => {
    const root = probe()
    expect(paintOf(root, 'background-color', 'var(--color-admin-card)')).toBe(
      'rgba(255, 255, 255, 0.94)',
    )
    expect(paintOf(root, 'color', 'var(--color-admin-text-1)')).toBe('rgb(49, 49, 49)')
    expect(paintOf(root, 'background-color', 'var(--color-board-surface)')).toBe(
      'rgba(255, 255, 255, 0.9)',
    )
  })

  it('resolves shared radius roles to their pixel radii', () => {
    const root = probe()
    expect(utilityPaint(root, 'rounded-control', 'border-radius')).toBe('8px')
    expect(utilityPaint(root, 'rounded-panel', 'border-radius')).toBe('12px')
    expect(utilityPaint(root, 'rounded-overlay', 'border-radius')).toBe('16px')
  })

  it('emits native stacking, shadow, and type utilities', () => {
    const root = probe()
    expect(utilityPaint(root, 'z-banner', 'z-index')).toBe('1200')
    expect(utilityPaint(root, 'z-admin-dialog', 'z-index')).toBe('50')
    expect(utilityPaint(root, 'shadow-board-dialog', 'box-shadow')).toContain(
      'rgba(0, 0, 0, 0.12) 0px 22px 58px 0px',
    )
    expect(utilityPaint(root, 'type-body', 'font-size')).toBe('13px')
    expect(utilityPaint(root, 'text-admin-body', 'font-size')).toBe('13px')
  })

  it('resolves domain dark roles on the default root', () => {
    document.documentElement.classList.remove('light')
    const root = probe()
    expect(paintOf(root, 'background-color', 'var(--color-admin-bg)')).toBe('rgb(15, 15, 15)')
    expect(paintOf(root, 'background-color', 'var(--color-board-page)')).toBe('rgb(15, 15, 15)')
    expect(paintOf(root, 'background-color', 'var(--color-canvas-node-bg)')).toBe('rgb(29, 29, 29)')
  })
})
