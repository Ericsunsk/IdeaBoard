import type { Component } from 'vue'
import {
  Brightness,
  ColorFilter,
  EditPencil,
  MediaImage,
  PrecisionTool,
  ReportsSolid,
  SineWave,
  VerticalMerge,
} from '@iconoir/vue'
import type { ImageRetouchKind, RetouchEffectLayer, RetouchLayer } from '../core/model.ts'

/*
 * Thin retouch state-view model (third-wave S8, behavior zero change).
 *
 * ADR-0013 note: the icon-coupled remainder lives here in session/ because it
 * imports @iconoir/vue components. The pure kind label table + rail order now
 * live in core/kindLabels.ts — import that module directly for labels/rail
 * order; this view model only consumes them to assemble the icon rail.
 * The tables below stay typed Record<ImageRetouchKind, ...>, so a kind added
 * to IMAGE_RETOUCH_DEFINITIONS forces a compile error here, and the unit spec
 * pins the rail order against the live definition keys. i18n stores the
 * labelKey only and translates at call time with t() inside a computed, the
 * same runtime re-translation precedent CurvesControl uses for its preset
 * labels (see curvePresets) — nothing is baked at module load, so locale
 * switches stay reactive.
 */

import { RETOUCH_EFFECT_RAIL_ORDER, RETOUCH_KIND_LABEL_KEYS } from '../core/kindLabels.ts'

export const RETOUCH_KIND_ICONS: Record<ImageRetouchKind, Component> = {
  hsl: ColorFilter,
  threshold: Brightness,
  curves: SineWave,
  levels: ReportsSolid,
  sharpen: PrecisionTool,
}

export type RetouchEffectRailItem = {
  kind: ImageRetouchKind
  labelKey: string
  icon: Component
}

export const RETOUCH_EFFECT_RAIL: readonly RetouchEffectRailItem[] = RETOUCH_EFFECT_RAIL_ORDER.map(
  (kind) => ({
    kind,
    labelKey: RETOUCH_KIND_LABEL_KEYS[kind],
    icon: RETOUCH_KIND_ICONS[kind],
  }),
)

/*
 * Generic thumb for forward-compat kinds: an unknown effect kind degrades
 * to the image glyph instead of throwing (LayersPanel used to assertNever).
 */
export const RETOUCH_UNKNOWN_KIND_ICON: Component = MediaImage

export function resolveRetouchKindIcon(kind: ImageRetouchKind): Component {
  return RETOUCH_KIND_ICONS[kind] ?? RETOUCH_UNKNOWN_KIND_ICON
}

export function resolveRetouchLayerKindKey(layer: RetouchLayer): string {
  if (layer.type === 'source') return 'source'
  if (layer.type === 'effect') return layer.kind
  return layer.type
}

export function resolveRetouchLayerIcon(layer: RetouchLayer): Component {
  if (layer.type === 'source') return MediaImage
  if (layer.type === 'paint') return EditPencil
  if (layer.type === 'merged') return VerticalMerge
  return resolveRetouchKindIcon(layer.kind)
}

export type RetouchChromeSelection = {
  selectedKind: ImageRetouchKind | null
  selectedLayerId: string
}

/*
 * Chrome selection consistency: the rail highlight follows the single
 * editing identity (pending preview included) while the layer list cursor
 * follows the tree cursor. Both resolve here so EffectActions and
 * LayersPanel cannot drift apart (deriving the kind from layers +
 * selectedLayerId would blank the rail while a pending kind is staged).
 */
export function resolveRetouchChromeSelection(input: {
  selectedEffectLayer: RetouchEffectLayer | null
  selectedLayerId: string
}): RetouchChromeSelection {
  return {
    selectedKind: input.selectedEffectLayer?.kind ?? null,
    selectedLayerId: input.selectedLayerId,
  }
}
