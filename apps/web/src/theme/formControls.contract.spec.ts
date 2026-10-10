/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { ibFormInputClass, ibFormInputErrorClass } from './formControls'

const src = (...parts: string[]) => readFileSync(resolve(process.cwd(), 'src', ...parts), 'utf8')

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

  it('wires the redeem input to the error-text contract', () => {
    const pane = src('features/account/presentation/rewards', 'RewardsPane.vue')
    expect(pane).toContain('aria-describedby="redeemFailed')
    expect(pane).toContain('id="account-reward-code-error"')
    expect(pane).toContain('role="alert"')
  })

  it('gives the canvas prompt textarea a visible focus ring', () => {
    // Borderless and transparent by design, so the ring is the only focus
    // indication keyboard users get.
    const composer = src(
      'features/canvas/presentation/nodes/primitives',
      'PromptComposerControls.vue',
    )
    expect(composer).toContain('focus-visible:ring-ring/20')
  })

  it('keeps the shared error chrome on the user danger tokens', () => {
    // `!` is load-bearing: the base chrome's hover border (0,2,0) otherwise
    // beats the plain error border (0,1,0) and hides the error under the mouse.
    expect(ibFormInputErrorClass).toContain('!border-destructive')
    expect(ibFormInputErrorClass).toContain('focus:bg-destructive/5')
  })

  it('keeps the Admin invalid border above hover and focus too', () => {
    for (const file of [
      ['features/admin/shared/components', 'AdminInput.vue'],
      ['features/admin/shared/components', 'AdminNumberInput.vue'],
      ['features/admin/payments/stripe-settings', 'StripeSettingsDialog.vue'],
    ]) {
      expect(src(...file)).toContain('!border-destructive')
    }
  })
})
