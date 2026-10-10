import { describe, expect, it } from 'vitest'
import {
  RETOUCH_CURVE_ENDPOINT_BLACK_ID,
  RETOUCH_CURVE_ENDPOINT_WHITE_ID,
  RETOUCH_CURVE_POINT_PREFIX,
  RETOUCH_CURVE_PRESET_PREFIX,
  RETOUCH_EFFECT_LAYER_PREFIX,
  RETOUCH_LEVELS_HIGHLIGHTS_MARKER_ID,
  RETOUCH_LEVELS_MIDTONES_MARKER_ID,
  RETOUCH_LEVELS_OUTPUT_BLACK_MARKER_ID,
  RETOUCH_LEVELS_OUTPUT_WHITE_MARKER_ID,
  RETOUCH_LEVELS_SHADOWS_MARKER_ID,
  RETOUCH_MERGED_LAYER_PREFIX,
  RETOUCH_PAINT_LAYER_PREFIX,
  RETOUCH_SOURCE_LAYER_ID,
  RETOUCH_THRESHOLD_MARKER_ID,
  makeCurvePointFallbackId,
  makeCurvePointId,
  makeRetouchLayerId,
} from './ids'
import { SOURCE_LAYER_ID, makeRetouchLayerId as makeRetouchLayerIdFromStack } from './layers/layerStack'

describe('ids', () => {
  it('keeps layer prefixes and the source id stable', () => {
    expect(RETOUCH_SOURCE_LAYER_ID).toBe('source')
    expect(SOURCE_LAYER_ID).toBe(RETOUCH_SOURCE_LAYER_ID)
    expect(RETOUCH_EFFECT_LAYER_PREFIX).toBe('retouch:')
    expect(RETOUCH_PAINT_LAYER_PREFIX).toBe('paint:')
    expect(RETOUCH_MERGED_LAYER_PREFIX).toBe('merged:')
  })

  it('keeps curve endpoint, point, and preset ids stable', () => {
    expect(RETOUCH_CURVE_ENDPOINT_BLACK_ID).toBe('endpoint:black')
    expect(RETOUCH_CURVE_ENDPOINT_WHITE_ID).toBe('endpoint:white')
    expect(RETOUCH_CURVE_POINT_PREFIX).toBe('point:')
    expect(RETOUCH_CURVE_PRESET_PREFIX).toBe('preset:')
  })

  it('keeps levels and threshold marker ids stable', () => {
    expect(RETOUCH_THRESHOLD_MARKER_ID).toBe('threshold')
    expect(RETOUCH_LEVELS_SHADOWS_MARKER_ID).toBe('shadows')
    expect(RETOUCH_LEVELS_MIDTONES_MARKER_ID).toBe('midtones')
    expect(RETOUCH_LEVELS_HIGHLIGHTS_MARKER_ID).toBe('highlights')
    expect(RETOUCH_LEVELS_OUTPUT_BLACK_MARKER_ID).toBe('outputBlack')
    expect(RETOUCH_LEVELS_OUTPUT_WHITE_MARKER_ID).toBe('outputWhite')
  })

  it('generates unique layer ids per scope', () => {
    const first = makeRetouchLayerId('paint')
    const second = makeRetouchLayerId('paint')
    expect(first.startsWith(RETOUCH_PAINT_LAYER_PREFIX)).toBe(true)
    expect(second.startsWith(RETOUCH_PAINT_LAYER_PREFIX)).toBe(true)
    expect(first).not.toBe(second)
    expect(makeRetouchLayerIdFromStack('merged').startsWith(RETOUCH_MERGED_LAYER_PREFIX)).toBe(true)
  })

  it('generates curve point ids under the shared point prefix', () => {
    const randomId = makeCurvePointId(50)
    expect(randomId.startsWith(RETOUCH_CURVE_POINT_PREFIX)).toBe(true)
    expect(randomId).toContain(':50')
    expect(makeCurvePointFallbackId(42)).toBe(`${RETOUCH_CURVE_POINT_PREFIX}42`)
  })
})
