/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

/*
 * Retouch shared-token contract (residual).
 *
 * The cross-component sharing pins (plot frame, control points, panel
 * metrics, action row, preview/commit wiring, marker selectors, kind map,
 * view-model rail, barrel alias) now live in
 * `scripts/checkRetouchSharedContract.ts` (`lint:retouch-shared-contract`
 * wiring pending), which proves each rule armed with negative fixtures.
 * What stays here: the focus-ring assertions awaiting the browser
 * replacement (`computed-style/3a1e8303`) and the locale assertions frozen
 * under `i18n/retouch-call-time-locale-switch` (tool decision pending).
 *
 * Behavioral coverage (listbox/slider focus, marker keyboard, preset/channel
 * switching) lives in the component specs via true rendering assertions, not
 * here. Color enforcement lives in stylelint + checkStyleGovernance
 * (raw-color/hex budgets), not in negated source-string assertions.
 */

const uiRoot = resolve(process.cwd(), 'src/features/canvas/tools/retouch/ui')
const readComponent = (relativePath: string) => readFileSync(resolve(uiRoot, relativePath), 'utf8')

const histogramPlot = readComponent('shared/styles/retouch-histogram-plot.css')
const curves = readComponent('shared/styles/retouch-curves.css')
const effectActions = readComponent('chrome/EffectActions.vue')
const layersPanel = readComponent('layers/LayersPanel.vue')
const thresholdControl = readComponent('threshold/ThresholdControl.vue')
const levelsControl = readComponent('levels/LevelsControl.vue')
const curvesControl = readComponent('curves/CurvesControl.vue')
const levelsFields = readComponent('levels/LevelsFields.vue')
const processingRoot = resolve(process.cwd(), 'src/features/canvas/tools/retouch/core')
const readProcessing = (relativePath: string) =>
  readFileSync(resolve(processingRoot, relativePath), 'utf8')
const levelsProcessing = readProcessing('kinds/levels.ts')
const thresholdProcessing = readProcessing('kinds/threshold.ts')
const curvesProcessing = readProcessing('kinds/curves.ts')

describe('retouch readability and keyboard contracts', () => {
  it('keeps focus rings on motion-safe outline tokens without layout shift', () => {
    expect(histogramPlot).toContain('--focus-outline-width')
    expect(histogramPlot).toContain('outline-offset')
    expect(curves).toContain('--focus-outline-width')
    expect(curves).toContain('--focus-outline-color')
    expect(layersPanel).toContain('--focus-outline-offset')
  })
})

describe('retouch state view model', () => {
  const viewModel = readFileSync(
    resolve(process.cwd(), 'src/features/canvas/tools/retouch/session/retouchViewModel.ts'),
    'utf8',
  )

  it('derives kind icon/label/rail tables from definitions with call-time t()', () => {
    expect(viewModel).toContain('RETOUCH_KIND_LABEL_KEYS')
    expect(viewModel).toContain('labelKey')
    // Labels translate at call time (the CurvesControl runtime precedent):
    // the view model never bakes t() at module load.
    expect(viewModel).not.toContain('useI18n')
    expect(viewModel).not.toContain('i18n.global.t')
    expect(effectActions).toContain('t(item.labelKey)')
  })
})

describe('retouch marker and preset labelKeys', () => {
  it('keeps levels/threshold markers and curve presets locale-free with call-time t()', () => {
    // Processing carries only labelKeys (same precedent as definitions ranges
    // / S8 rail); components translate at render time via t(labelKey) so
    // runtime locale switches stay reactive. Nothing is baked at module load
    // or call time via the i18n singleton.
    expect(levelsProcessing).toContain('labelKey')
    expect(levelsProcessing).not.toContain('i18n.global.t')
    expect(thresholdProcessing).toContain('labelKey')
    expect(thresholdProcessing).not.toContain('i18n.global.t')
    expect(curvesProcessing).toContain('labelKey')
    expect(curvesProcessing).not.toContain('i18n.global.t')
    expect(curvesProcessing).not.toContain('label: i18n')
    expect(levelsControl).toContain('t(marker.labelKey')
    expect(thresholdControl).toContain('t(marker.labelKey')
    expect(curvesControl).toContain('t(preset.labelKey)')
    expect(curvesControl).not.toContain('CURVE_PRESET_LABEL_KEYS')
    expect(levelsFields).toContain('LEVEL_FIELD_LABEL_KEYS')
    expect(levelsFields).toContain('t(labelKey)')
  })
})
