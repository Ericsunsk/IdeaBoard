import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { defineComponent, h, nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../../../../test/render'
import { setUserLocale, type SupportedLocale } from '../../../../../../core/i18n'
import EffectActions from './EffectActions.vue'

/*
 * Retouch rail four-locale switch (i18n/retouch-call-time-locale-switch).
 *
 * Copies the proven fourLocaleServiceCopy.contract.spec.ts pattern: switch the
 * locale at call time, assert each locale renders distinct copy, no remount.
 * The rail carries only labelKeys from core/kindLabels.ts (framework-free, no
 * i18n runtime import); EffectActions runs t(item.labelKey) inside a computed
 * so runtime locale switches stay reactive. A static-label regression (baked
 * strings at module load) would collapse every locale to one copy and fail
 * the distinct-set assertion below.
 */

const ALL_LOCALES: SupportedLocale[] = ['en', 'zh-CN', 'es', 'ko']

// Rail order is the legacy threshold/hsl/sharpen/curves/levels order; each row
// is canvas.retouch.effects.<kind> in the active locale.
const RAIL_LABELS: Record<SupportedLocale, readonly string[]> = {
  en: ['Threshold', 'Hue/Saturation', 'Sharpen', 'Curves', 'Levels'],
  'zh-CN': ['阈值', '色相/饱和度', '锐化', '曲线', '色阶'],
  es: ['Umbral', 'Tono/Saturación', 'Enfocar', 'Curvas', 'Niveles'],
  ko: ['임계값', '색조/채도', '샤프닝', '커브', '레벨'],
}

function renderRail() {
  const Harness = defineComponent({
    setup() {
      return () => h(EffectActions, { selectedKind: null })
    },
  })
  return renderWithUser(Harness)
}

function railAriaLabels(): string[] {
  return screen
    .getAllByRole('button')
    .map((button) => button.getAttribute('aria-label') ?? '')
}

describe('retouch rail four-locale switch (i18n/retouch-call-time-locale-switch)', () => {
  afterEach(async () => {
    vi.restoreAllMocks()
    await setUserLocale('en')
  })

  it('keeps kind labels locale-free in core without importing the i18n runtime', () => {
    const kindLabels = readFileSync(
      resolve(process.cwd(), 'src/features/canvas/tools/retouch/core/kindLabels.ts'),
      'utf8',
    )
    expect(kindLabels).toContain('RETOUCH_KIND_LABEL_KEYS')
    expect(kindLabels).toContain('RETOUCH_EFFECT_RAIL_ORDER')
    expect(kindLabels).toContain('labelKey')
    expect(kindLabels).not.toContain('useI18n')
    expect(kindLabels).not.toContain('vue-i18n')
    expect(kindLabels).not.toContain('i18n.global')
  })

  it('re-translates every rail kind label across all four locales without remount', async () => {
    renderRail()

    // English baseline before any switch.
    expect(railAriaLabels()).toEqual([...RAIL_LABELS.en])
    const firstButtons = screen.getAllByRole('button')
    expect(firstButtons).toHaveLength(RAIL_LABELS.en.length)

    const rendered: string[] = [railAriaLabels().join('|')]
    for (const locale of ALL_LOCALES.filter((item) => item !== 'en')) {
      await expect(setUserLocale(locale)).resolves.toBe(true)
      await nextTick()

      // Same five buttons, patched in place: locale switches must not remount
      // the rail (v-for keys are stable kinds).
      const buttons = screen.getAllByRole('button')
      expect(buttons).toHaveLength(RAIL_LABELS.en.length)
      buttons.forEach((button, index) => {
        expect(button).toBe(firstButtons[index])
      })
      expect(railAriaLabels()).toEqual([...RAIL_LABELS[locale]])
      rendered.push(railAriaLabels().join('|'))
    }

    // Static baked labels would collapse every switch to the English copy.
    expect(new Set(rendered).size).toBe(ALL_LOCALES.length)

    // Switching back to English restores the baseline on the same nodes.
    await expect(setUserLocale('en')).resolves.toBe(true)
    await nextTick()
    expect(railAriaLabels()).toEqual([...RAIL_LABELS.en])
    screen
      .getAllByRole('button')
      .forEach((button, index) => {
        expect(button).toBe(firstButtons[index])
      })
  })

  it('renders four distinct rail copies, one per locale', async () => {
    renderRail()

    const rendered: string[] = []
    for (const locale of ALL_LOCALES) {
      await expect(setUserLocale(locale)).resolves.toBe(true)
      await nextTick()
      const labels = railAriaLabels()
      expect(labels).toEqual([...RAIL_LABELS[locale]])
      rendered.push(labels.join('|'))
    }
    // Every switch must actually change the visible copy; an English fallback
    // for a missing locale bundle would collapse these to one string.
    expect(new Set(rendered).size).toBe(ALL_LOCALES.length)
  })
})
