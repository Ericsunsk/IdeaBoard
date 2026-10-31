/// <reference types="node" />

import { describe, expect, it } from 'vitest'
import { ibFormInputClass, ibFormInputErrorClass } from './formControls'

/**
 * User-side form inputs (everything except the search bar) keep the Admin
 * field contract: one height, a visible focus ring, persistent error text
 * below the control, and an invalid state on the control itself.
 *
 * The user side drifted on all four: auth inputs were h-10 while account panes
 * were h-12, `ibFormInputClass` had `outline-none` with no ring replacement
 * (keyboard focus was a faint border shift), required-field errors were swapped
 * into the placeholder where they vanished on first keystroke, and the redeem
 * input surfaced submit failures only as a toast with no control state.
 */
describe('User form input contract', () => {
  it('keeps one height and a visible focus ring on the shared input chrome', () => {
    // h-10 everywhere on the user side; the ring reuses the standard shadcn focus ring token
    expect(ibFormInputClass).toContain(' h-10 ')
    expect(ibFormInputClass).not.toContain('h-12')
    expect(ibFormInputClass).toContain('focus-visible:ring-ring')
    expect(ibFormInputClass).not.toContain('focus:border-primary')
    expect(ibFormInputClass).not.toContain('hover:border-border-hover')
  })

  // P3 retirement: the "wires the redeem input to the error-text contract"
  // source-string assertions (3) moved to the
  // source-policy/formControls.contract/5d0f9348 governance script
  // (scripts/checkThemeContractRetirements.ts, rule redeem-error-wiring),
  // which asserts the aria-describedby -> id -> role="alert" linkage.

  // P5 retirement (computed-style/fef79256): the "gives the canvas prompt
  // textarea a visible focus ring" source-string assertion is replaced by
  // the computed-style browser contract
  // (`features/canvas/ui/nodes/primitives/composer/promptComposerRing.browser.spec.ts`),
  // which asserts the rendered textarea ring paint rides the live ring token.

  it('keeps the shared error chrome on the user danger tokens', () => {
    // `!` is load-bearing: the base chrome's hover border (0,2,0) otherwise
    // beats the plain error border (0,1,0) and hides the error under the mouse.
    expect(ibFormInputErrorClass).toContain('!border-destructive')
    expect(ibFormInputErrorClass).toContain('focus:bg-destructive/5')
  })

  // P4 retirement (source-f5f7dc876e706d72-1): the "keeps the Admin invalid
  // border above hover and focus too" source-string assertion is replaced by
  // the computed-style browser contract
  // (`features/admin/credits/ui/shared/invalidBorder.browser.spec.ts`), which
  // asserts the rendered invalid border paint beats hover and focus through
  // the live destructive token.
})
