import '@testing-library/jest-dom/vitest'

// Admin invalid-border computed-style browser contract.
//
// Observable replacement for the retired
// `src/theme/formControls.contract.spec.ts` source-string assertion
// (`source-f5f7dc876e706d72-1`: `expect(src(...file)).toContain('!border-destructive')`).
// Asserts rendered output — computed border paint on the real Admin input
// chrome — never file text. Covers the Admin invalid pattern shared by
// `AdminInput`, `AdminNumberInput`, and `StripeSettingsDialog` via the
// canonical `AdminInput[invalid]` node: the invalid border beats the rest
// border and survives hover and keyboard focus.

import { page, userEvent as browserUser } from 'vitest/browser'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { renderWithUser } from '../../../../../test/render'
import { AdminInput } from '../../../shared'
import '../../../../../app/style.css'

function styleOf(element: Element): CSSStyleDeclaration {
  return window.getComputedStyle(element)
}

describe('Admin invalid border browser contract', () => {
  beforeEach(async () => {
    await page.viewport(1280, 800)
    document.documentElement.classList.remove('dark')
  })

  afterEach(async () => {
    await browserUser.hover(document.body)
  })

  it('keeps the Admin invalid border above hover and focus', async () => {
    const rendered = renderWithUser(AdminInput, {
      props: { invalid: true, ariaLabel: 'Admin invalid input' },
    })
    try {
      const input = rendered.container.querySelector('input') as HTMLElement
      expect(input).toBeTruthy()

      // Expected paint rides the live destructive token through a probe, so a
      // hardcoded Admin-side value cannot satisfy this.
      const destructiveProbe = document.createElement('div')
      destructiveProbe.style.borderStyle = 'solid'
      destructiveProbe.style.borderColor = 'var(--color-destructive)'
      document.body.append(destructiveProbe)
      const restProbe = document.createElement('div')
      restProbe.style.borderStyle = 'solid'
      restProbe.style.borderColor = 'var(--color-input)'
      document.body.append(restProbe)
      try {
        expect(styleOf(input).borderStyle).toBe('solid')
        // Invalid beats the rest input border.
        expect(styleOf(input).borderColor).toBe(styleOf(destructiveProbe).borderColor)
        expect(styleOf(input).borderColor).not.toBe(styleOf(restProbe).borderColor)

        // Provenance: the invalid rule tracks the destructive token live —
        // moving the token moves the input paint, so a hardcoded invalid
        // value (or a dropped `!border-destructive` rule) cannot satisfy this.
        document.documentElement.style.setProperty('--color-destructive', 'rgb(255, 0, 0)')
        try {
          await expect.poll(() => styleOf(input).borderColor).toBe('rgb(255, 0, 0)')
        } finally {
          document.documentElement.style.removeProperty('--color-destructive')
        }
        await expect
          .poll(() => styleOf(input).borderColor)
          .toBe(styleOf(destructiveProbe).borderColor)

        // Hover never drops the invalid border.
        await browserUser.hover(input)
        expect(input.matches(':hover')).toBe(true)
        await expect
          .poll(() => styleOf(input).borderColor)
          .toBe(styleOf(destructiveProbe).borderColor)

        // Keyboard focus never drops the invalid border either.
        input.focus()
        expect(document.activeElement).toBe(input)
        expect(styleOf(input).borderColor).toBe(styleOf(destructiveProbe).borderColor)
      } finally {
        destructiveProbe.remove()
        restProbe.remove()
      }
    } finally {
      rendered.unmount()
    }
  })
})
