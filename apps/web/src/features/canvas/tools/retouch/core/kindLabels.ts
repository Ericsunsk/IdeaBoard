import type { ImageRetouchKind } from './model.ts'

/*
 * Pure kind label table for the retouch scope (ADR-0013 core/).
 *
 * Framework-free: no vue reactivity, no icons, no i18n calls. Labels stay as
 * labelKeys so components translate at call time with t() (same precedent as
 * CurvesControl presets). The icon-coupled remainder (RETOUCH_KIND_ICONS,
 * RETOUCH_EFFECT_RAIL assembly, resolveRetouchKindIcon/LayerIcon) stays in
 * session/retouchViewModel.ts — see that module's note — which imports this
 * table to assemble the rail.
 */

export const RETOUCH_KIND_LABEL_KEYS: Record<ImageRetouchKind, string> = {
  hsl: 'canvas.retouch.effects.hsl',
  threshold: 'canvas.retouch.effects.threshold',
  curves: 'canvas.retouch.effects.curves',
  levels: 'canvas.retouch.effects.levels',
  sharpen: 'canvas.retouch.effects.sharpen',
}

/*
 * Rail order is a view concern (EffectActions layout), not a definition
 * concern: it keeps the legacy threshold/hsl/sharpen/curves/levels order so
 * the migration changes no pixels. The kind *set* still derives from
 * definitions — the view-model spec asserts the rail covers exactly the
 * definition keys.
 */
export const RETOUCH_EFFECT_RAIL_ORDER: readonly ImageRetouchKind[] = [
  'threshold',
  'hsl',
  'sharpen',
  'curves',
  'levels',
]
