/// <reference types="node" />

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

/*
 * Retouch shared-token contract (T7 minimal set).
 *
 * Pins only the cross-component sharing that must not drift: one plot frame,
 * one control-point definition, one action row, panel metrics from the Canvas
 * surface contract, preview/commit wiring, listbox/slider a11y markers, the
 * histogram marker selector folding, the properties-panel kind map, the
 * view-model rail, and the barrel alias.
 *
 * Behavioral coverage (listbox/slider focus, marker keyboard, preset/channel
 * switching) lives in the component specs via true rendering assertions, not
 * here. Color enforcement lives in stylelint + checkStyleGovernance
 * (raw-color/hex budgets), not in negated source-string assertions.
 */

const uiRoot = resolve(process.cwd(), 'src/features/canvas/tools/retouch/ui')
const readComponent = (relativePath: string) => readFileSync(resolve(uiRoot, relativePath), 'utf8')

const plotFrame = readComponent('shared/styles/retouch-plot-frame.css')
const histogramPlot = readComponent('shared/styles/retouch-histogram-plot.css')
const curves = readComponent('shared/styles/retouch-curves.css')
const histogramControls = readComponent('shared/styles/retouch-histogram-controls.css')
const controlPoints = readComponent('shared/styles/retouch-control-points.css')
const panel = readComponent('shared/styles/retouch-panel.css')
const layerActions = readComponent('layers/LayerActions.vue')
const effectActions = readComponent('chrome/EffectActions.vue')
const layersPanel = readComponent('layers/LayersPanel.vue')
const histogramPlotVue = readComponent('histogram/HistogramPlot.vue')
const chrome = readComponent('chrome/Chrome.vue')
const thresholdControl = readComponent('threshold/ThresholdControl.vue')
const levelsControl = readComponent('levels/LevelsControl.vue')
const curvesControl = readComponent('curves/CurvesControl.vue')
const levelsFields = readComponent('levels/LevelsFields.vue')
const propertiesPanel = readComponent('chrome/PropertiesPanel.vue')
const retouchBarrel = readFileSync(resolve(uiRoot, '../index.ts'), 'utf8')
const processingRoot = resolve(process.cwd(), 'src/features/canvas/tools/retouch/core')
const readProcessing = (relativePath: string) =>
  readFileSync(resolve(processingRoot, relativePath), 'utf8')
const levelsProcessing = readProcessing('kinds/levels.ts')
const thresholdProcessing = readProcessing('kinds/threshold.ts')
const curvesProcessing = readProcessing('kinds/curves.ts')

describe('retouch shared plot frame', () => {
  it('serves both histogram and curves frames from one contract', () => {
    expect(histogramPlot).toContain("@import './retouch-plot-frame.css'")
    expect(curves).toContain("@import './retouch-plot-frame.css'")
    expect(plotFrame).toContain('.retouch-histogram-plot')
    expect(plotFrame).toContain('.retouch-curves__graph')
    expect(plotFrame).toContain('--retouch-plot-frame-radius:')
  })

  it('keeps the veil and histogram wash on Canvas surface tokens', () => {
    expect(plotFrame).toContain('--canvas-glass-bg')
    expect(plotFrame).toContain('--canvas-surface-muted')
    expect(plotFrame).toContain('--canvas-text-subtle')
  })

  it('leaves grid strokes and marker accents on the surface contract', () => {
    expect(plotFrame).toContain('--canvas-surface-border-soft')
    expect(plotFrame).toContain('--canvas-accent-outline')
    expect(histogramPlot).toContain('--canvas-accent-strong')
    expect(histogramControls).toContain('--canvas-accent-outline')
  })
})

describe('retouch shared control points', () => {
  it('defines handle sizing once and reuses it in curves and histogram markers', () => {
    expect(controlPoints).toContain('--retouch-control-point-size:')
    expect(controlPoints).toContain('--retouch-control-point-bg:')
    expect(curves).toContain('var(--retouch-control-point-bg)')
    expect(histogramPlot).toContain('var(--retouch-control-point-bg')
    expect(histogramControls).toContain('var(--retouch-control-point-')
  })

  it('rounds instrument dots with the pill token', () => {
    expect(histogramPlot).toContain('var(--radius-pill)')
    expect(curves).toContain('var(--radius-pill)')
  })
})

describe('retouch shared panel shell and action row', () => {
  it('sources panel metrics from the Canvas surface contract', () => {
    expect(panel).toContain('--retouch-panel-inset: var(--canvas-tool-button-gap)')
    expect(panel).toContain('--retouch-panel-control-radius: var(--canvas-radius-md)')
    expect(panel).toContain('--retouch-panel-text-size: var(--canvas-font-size-caption)')
    expect(panel).toContain('.retouch-action-row')
    expect(panel).toContain('repeat(5, var(--node-tool-button-size))')
  })

  it('keeps both button groups on the shared action row', () => {
    expect(layerActions).toContain('retouch-action-row')
    expect(effectActions).toContain('retouch-action-row')
  })

  it('derives the layer thumb radius from the panel control radius', () => {
    expect(layersPanel).toContain('calc(var(--retouch-panel-control-radius) - 3px)')
  })
})

describe('retouch preview/commit distinction', () => {
  it('keeps apply as the single primary commit control without local overrides', () => {
    expect(layerActions).toContain('primary')
    expect(layerActions).toContain(':disabled="applyDisabled"')
    expect(layerActions).toContain("emit('apply-retouch')")
  })

  it('keeps Chrome wiring preview state to the commit control', () => {
    expect(chrome).toContain(':apply-disabled')
    expect(chrome).toContain('@apply-retouch="emit(\'apply-retouch\')"')
    expect(chrome).toContain(':histogram-source')
  })

  it('injects the apply gate from the parent instead of reading the stack directly', () => {
    // Same gate as commands.readApplySource: the adapter computes canApply
    // from hasRenderableRetouchLayerStack, Chrome only binds it.
    expect(chrome).toContain('canApply: boolean')
    expect(chrome).toContain('!canApply || applyingRetouch')
    expect(chrome).not.toContain('hasRenderableRetouchLayerStack')
    expect(chrome).not.toContain('canApplyRetouch')
  })
})

describe('retouch readability and keyboard contracts', () => {
  it('keeps listbox, slider, and tooltip markers that the behavioral specs pin', () => {
    expect(layersPanel).toContain('role="listbox"')
    expect(layersPanel).toContain('hit-target-32-inline')
    expect(layersPanel).toContain('canvas-tool-tooltip-trigger')
    expect(histogramPlotVue).toContain('role="slider"')
    expect(histogramPlot).toContain(':focus-visible')
    expect(curves).toContain(':focus-visible')
  })

  it('keeps focus rings on motion-safe outline tokens without layout shift', () => {
    expect(histogramPlot).toContain('--focus-outline-width')
    expect(histogramPlot).toContain('outline-offset')
    expect(curves).toContain('--focus-outline-width')
    expect(curves).toContain('--focus-outline-color')
    expect(layersPanel).toContain('--focus-outline-offset')
  })
})

describe('retouch histogram marker sharing', () => {
  it('folds the threshold/levels marker pair with zero-specificity :where()', () => {
    expect(histogramControls).toContain(
      ':where(.retouch-threshold-control, .retouch-levels-control)',
    )
    expect(histogramControls).toContain('.retouch-histogram-plot__marker')
    expect(histogramControls).toContain('var(--retouch-control-point-')
  })

  it('keeps marker modes distinct across Levels button and Threshold range', () => {
    expect(thresholdControl).toContain('marker-mode="range"')
    expect(levelsControl).not.toContain('marker-mode="range"')
  })
})

describe('retouch properties panel mapping', () => {
  it('renders the four effect controls from one kind map behind component :is', () => {
    expect(propertiesPanel).toContain('RETOUCH_CONTROL_COMPONENTS')
    expect(propertiesPanel).toContain('component')
    expect(propertiesPanel).toContain(':is="activeControl"')
    expect(propertiesPanel).toContain('threshold')
    expect(propertiesPanel).toContain('curves')
    expect(propertiesPanel).toContain('levels')
    expect(propertiesPanel).toContain('sharpen')
    expect(propertiesPanel).toContain(':histogram-source')
  })

  it('keeps the hsl branch and the sharpen histogram exception', () => {
    expect(propertiesPanel).toContain("kind === 'hsl'")
    expect(propertiesPanel).toContain("kind === 'sharpen'")
  })
})

describe('retouch state view model', () => {
  const viewModel = readFileSync(
    resolve(process.cwd(), 'src/features/canvas/tools/retouch/session/retouchViewModel.ts'),
    'utf8',
  )

  it('derives kind icon/label/rail tables from definitions with call-time t()', () => {
    expect(viewModel).toContain('RETOUCH_KIND_LABEL_KEYS')
    expect(viewModel).toContain('RETOUCH_KIND_ICONS')
    expect(viewModel).toContain('RETOUCH_EFFECT_RAIL')
    expect(viewModel).toContain('labelKey')
    // Labels translate at call time (the CurvesControl runtime precedent):
    // the view model never bakes t() at module load.
    expect(viewModel).not.toContain('useI18n')
    expect(viewModel).not.toContain('i18n.global.t')
    expect(effectActions).toContain('RETOUCH_EFFECT_RAIL')
    expect(effectActions).toContain('t(item.labelKey)')
  })

  it('keeps LayersPanel table-driven with a generic fallback instead of throwing', () => {
    expect(viewModel).toContain('RETOUCH_UNKNOWN_KIND_ICON')
    expect(viewModel).toContain('resolveRetouchLayerIcon')
    expect(layersPanel).toContain('resolveRetouchLayerIcon')
    expect(layersPanel).not.toContain('assertNever')
    expect(layersPanel).not.toContain('Unhandled retouch layer kind')
  })

  it('resolves Chrome rail and list selection through one view-model function', () => {
    expect(viewModel).toContain('resolveRetouchChromeSelection')
    expect(chrome).toContain('resolveRetouchChromeSelection')
    expect(chrome).toContain('chromeSelection.selectedKind')
    expect(chrome).toContain('chromeSelection.selectedLayerId')
  })
})

describe('retouch barrel alias', () => {
  it('exposes Chrome as RetouchChrome through the unique exit', () => {
    expect(retouchBarrel).toContain('RetouchChrome')
    expect(retouchBarrel).toContain('./ui/chrome/Chrome.vue')
    expect(retouchBarrel).not.toMatch(/export\s*\{\s*default\s+as\s+Chrome\s*[,}]/)
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
    expect(levelsProcessing).not.toContain('ariaLabel:')
    expect(thresholdProcessing).toContain('labelKey')
    expect(thresholdProcessing).not.toContain('i18n.global.t')
    expect(thresholdProcessing).not.toContain('ariaLabel:')
    expect(curvesProcessing).toContain('labelKey')
    expect(curvesProcessing).not.toContain('i18n.global.t')
    expect(curvesProcessing).not.toContain('label: i18n')
    expect(levelsControl).toContain('t(marker.labelKey')
    expect(thresholdControl).toContain('t(marker.labelKey')
    expect(curvesControl).toContain('t(preset.labelKey)')
    expect(curvesControl).not.toContain('CURVE_PRESET_LABEL_KEYS')
    expect(levelsFields).toContain('LEVEL_FIELD_LABEL_KEYS')
    expect(levelsFields).toContain('computed(')
    expect(levelsFields).toContain('t(labelKey)')
  })
})
