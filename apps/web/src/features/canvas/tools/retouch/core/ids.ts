/**
 * Central ID + prefix source for the retouch feature (first-wave structural pass).
 *
 * core/*.ts must import layer / curve / marker ids from here instead of
 * hard-coding `retouch:` / `paint:` / `merged:` / `point:` / `preset:` literals.
 * ui/ controls import ids via core in the SaaS layout (KindTrait registry); until then the
 * previous export sites re-export these symbols for compatibility.
 */

/** Canonical id of the immutable source layer. */
export const RETOUCH_SOURCE_LAYER_ID = 'source'

/** Prefix for effect layer ids (`retouch:<kind>[:<seed>]`). */
export const RETOUCH_EFFECT_LAYER_PREFIX = 'retouch:'

/** Prefix for paint layer ids (`paint:<seed>`). */
export const RETOUCH_PAINT_LAYER_PREFIX = 'paint:'

/** Prefix for merged layer ids (`merged:<seed>`). */
export const RETOUCH_MERGED_LAYER_PREFIX = 'merged:'

/** Locked id of the black (input 0) curve endpoint. */
export const RETOUCH_CURVE_ENDPOINT_BLACK_ID = 'endpoint:black'

/** Locked id of the white (input 100) curve endpoint. */
export const RETOUCH_CURVE_ENDPOINT_WHITE_ID = 'endpoint:white'

/** Prefix for user-created curve point ids (`point:<suffix>`). */
export const RETOUCH_CURVE_POINT_PREFIX = 'point:'

/** Prefix for preset-owned curve point ids (`preset:<preset>:<slot>`). */
export const RETOUCH_CURVE_PRESET_PREFIX = 'preset:'

/** Marker id for the threshold handle. */
export const RETOUCH_THRESHOLD_MARKER_ID = 'threshold'

/** Marker id for the levels shadows handle. */
export const RETOUCH_LEVELS_SHADOWS_MARKER_ID = 'shadows'

/** Marker id for the levels midtones handle. */
export const RETOUCH_LEVELS_MIDTONES_MARKER_ID = 'midtones'

/** Marker id for the levels highlights handle. */
export const RETOUCH_LEVELS_HIGHLIGHTS_MARKER_ID = 'highlights'

/** Marker id for the levels output-black handle. */
export const RETOUCH_LEVELS_OUTPUT_BLACK_MARKER_ID = 'outputBlack'

/** Marker id for the levels output-white handle. */
export const RETOUCH_LEVELS_OUTPUT_WHITE_MARKER_ID = 'outputWhite'

let retouchLayerIdSeed = 0

/**
 * Sole factory for retouch layer ids. `scope` keeps the previous
 * `createLayerId` shape (`paint`, `merged`, `retouch:<kind>`); the seed suffix
 * guarantees uniqueness within the session.
 */
export function makeRetouchLayerId(scope: string): string {
  retouchLayerIdSeed += 1
  return `${scope}:${retouchLayerIdSeed}`
}

/**
 * Sole factory for random curve point ids. Keeps the `point:` prefix so specs
 * can assert the prefix instead of the timestamp payload.
 */
export function makeCurvePointId(input: number | string): string {
  return `${RETOUCH_CURVE_POINT_PREFIX}${Date.now().toString(36)}:${input}`
}

/** Deterministic fallback for curve points without a usable id (`point:<input>`). */
export function makeCurvePointFallbackId(input: number | string): string {
  return `${RETOUCH_CURVE_POINT_PREFIX}${input}`
}
