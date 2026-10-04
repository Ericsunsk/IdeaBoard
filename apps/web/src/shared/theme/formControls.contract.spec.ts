/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { ibFormInputClass, ibFormInputErrorClass } from './formControls'
import { inputVariants } from '../../components/ui/input/inputVariants'

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
    // h-10 everywhere on the user side; the ring reuses the same token the
    // user buttons already focus with.
    expect(ibFormInputClass).toContain(' h-10 ')
    expect(ibFormInputClass).not.toContain('h-12')
    expect(ibFormInputClass).toContain('focus-visible:ring-ring/20')
    // Border and glow share one modality, like Admin's focus-visible pair.
    expect(ibFormInputClass).toContain('focus-visible:border-primary')
    expect(ibFormInputClass).not.toContain('focus:border-primary')
  })

  it('leaves the focus border to the surface skins, never the primitive', () => {
    // The primitive's old `focus-visible:border-ring` tied at (0,2,0) with the
    // skins' solid accent border; stylesheet order then decided whether focus
    // showed a stroke or just a faint glow. Skins own focus, the primitive
    // stays neutral.
    const base = inputVariants({ size: 'default' })
    expect(base).not.toContain('border-ring')
    expect(base).not.toMatch(/focus(:-visible)?:border-/)
  })

  it('never renders an error as placeholder text', () => {
    // Placeholder-as-error disappears the moment the user types, and a
    // placeholder is not exposed as the accessible error either. Errors live
    // in persistent text below the control, like AdminFormField.
    for (const file of [
      ['features/auth/components/auth-fields', 'EmailField.vue'],
      ['features/auth/components/auth-fields', 'PasswordField.vue'],
      ['features/auth/components/auth-fields', 'OtpField.vue'],
      ['features/auth/components', 'RegisterAuthPanel.vue'],
    ]) {
      expect(src(...file)).not.toContain('inlineError')
    }
    expect(src('shared/components', 'IbFormField.vue')).not.toContain('inlineError')
  })

  it('always shows a persistent, announced error below the control', () => {
    const field = src('shared/components', 'IbFormField.vue')
    expect(field).toContain('role="alert"')
    // No branch may hide the message while an error is present.
    expect(field).not.toMatch(/v-else-if="shouldShowMessage"/)
    expect(field).toContain('v-else-if="invalid"')
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
      expect(src(...file)).toContain('!border-admin-danger')
    }
  })
})
