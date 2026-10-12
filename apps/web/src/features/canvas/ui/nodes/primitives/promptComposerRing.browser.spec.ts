import '@testing-library/jest-dom/vitest'

// Canvas prompt textarea ring computed-style browser contract.
//
// Observable replacement for the retired
// `src/theme/formControls.contract.spec.ts` "gives the canvas prompt textarea
// a visible focus ring" source-string assertion
// (`computed-style/fef79256`, 1 ID: `expect(composer).toContain(...)`):
// every retired constraint needs an effective computed-style replacement
// passing first. Follows the board/search P3 precedent
// (`boardCardMotion.browser.spec.ts`, `searchChromeStyle.browser.spec.ts`):
// assert rendered output — computed ring paint, live token provenance and no
// layout shift — never file text.
//
// Coverage map (ledger `src/shared/theme/formControls.contract.spec.ts`,
// scenario "gives the canvas prompt textarea a visible focus ring", line 79):
// - textarea ring (source-72d10c1416508e26-1): the borderless prompt textarea
//   rests without a ring and focuses to the shared `ring-ring/20` paint; the
//   paint tracks `--color-ring` live (moving it moves the ring, so a hardcoded
//   textarea-side value cannot satisfy this). The ring is box-shadow only, so
//   the border and the box never move. Reduced motion keeps the ring while the
//   motion tokens collapse to 0ms.

import { commands, page } from 'vitest/browser'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { cleanup } from '@testing-library/vue'
import { renderWithUser, screen } from '../../../../../test/render'
import { i18n } from '../../../../../core/i18n'
import '../../../../../app/style.css'
import PromptComposerControls from './PromptComposerControls.vue'

declare module 'vitest/internal/browser' {
  interface BrowserCommands {
    emulateMotion: (reducedMotion: 'reduce' | 'no-preference') => Promise<void>
  }
}

function styleOf(element: Element): CSSStyleDeclaration {
  return window.getComputedStyle(element)
}

async function setMotion(isReduced: boolean) {
  await commands.emulateMotion(isReduced ? 'reduce' : 'no-preference')
}

function renderComposer() {
  return renderWithUser(PromptComposerControls, {
    props: {
      modelValue: 'a poster of a harbor at dawn',
      outputCount: 1,
      placeholder: 'Describe the image',
      textareaClass: 'prompt-input',
      actionsClass: 'prompt-actions',
      sendButtonClass: 'prompt-send',
      showSubmit: false,
      showOutputCount: false,
      showOutputResolution: false,
      showAspectRatio: false,
    },
  })
}

describe('Canvas prompt textarea ring browser contract', () => {
  beforeEach(async () => {
    await page.viewport(1280, 800)
    document.documentElement.classList.remove('dark')
    i18n.global.locale.value = 'en'
    await setMotion(false)
  })

  afterEach(async () => {
    cleanup()
    document.body.innerHTML = ''
    await setMotion(false)
  })

  it('gives the borderless prompt textarea a visible ring on focus without layout shift', async () => {
    const rendered = renderComposer()
    try {
      const textarea = screen.getByRole('textbox') as HTMLElement
      // Borderless and transparent by design, so the ring is the only focus
      // indication keyboard users get.
      expect(styleOf(textarea).borderWidth).toBe('0px')
      expect(styleOf(textarea).boxShadow).toBe('none')
      const restBorder = styleOf(textarea).borderWidth
      textarea.focus()
      expect(document.activeElement).toBe(textarea)
      expect(textarea.matches(':focus-visible')).toBe(true)
      // The ring rides the 2px width the retired class pins; poll past the
      // control transition to its end state.
      await expect.poll(() => styleOf(textarea).boxShadow).not.toBe('none')
      expect(styleOf(textarea).boxShadow).toContain('2px')
      // Box-shadow only: the border never moves (outline would shift layout
      // on a borderless control; the ring does not).
      expect(styleOf(textarea).borderWidth).toBe(restBorder)
      // Provenance: the ring tracks the shared ring token live — moving it
      // moves the textarea paint, so a hardcoded textarea-side value (or a
      // dropped ring class) cannot satisfy this.
      const before = styleOf(textarea).boxShadow
      document.documentElement.style.setProperty('--color-ring', 'rgb(255, 0, 0)')
      try {
        await expect.poll(() => styleOf(textarea).boxShadow).not.toBe(before)
        expect(styleOf(textarea).boxShadow).toContain('2px')
      } finally {
        document.documentElement.style.removeProperty('--color-ring')
      }
      await expect.poll(() => styleOf(textarea).boxShadow).toBe(before)
    } finally {
      rendered.unmount()
    }
  })

  it('keeps the textarea ring under reduced motion while motion tokens collapse', async () => {
    await setMotion(true)
    expect(matchMedia('(prefers-reduced-motion: reduce)').matches).toBe(true)
    expect(
      styleOf(document.documentElement).getPropertyValue('--motion-control-duration').trim(),
    ).toBe('0ms')
    const rendered = renderComposer()
    try {
      const textarea = screen.getByRole('textbox') as HTMLElement
      textarea.focus()
      expect(textarea.matches(':focus-visible')).toBe(true)
      await expect.poll(() => styleOf(textarea).boxShadow).not.toBe('none')
      expect(styleOf(textarea).boxShadow).toContain('2px')
    } finally {
      rendered.unmount()
    }
  })
})
