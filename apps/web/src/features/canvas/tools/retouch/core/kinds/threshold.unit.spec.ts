import { describe, expect, it } from 'vitest'
import { i18n } from '../../../../../../core/i18n'
import {
  RETOUCH_THRESHOLD_MARKER_ID,
  clampThresholdValue,
  cloneThresholdDefaults,
  cloneThresholdParams,
  createThresholdMarker,
  isNeutralThresholdEffect,
  isThresholdParams,
  moveThresholdHandle,
  normalizeThresholdEffect,
} from './threshold'

describe('retouchThreshold', () => {
  it('clamps threshold values and falls back for non-finite input', () => {
    expect(clampThresholdValue(-10)).toBe(0)
    expect(clampThresholdValue(300)).toBe(255)
    expect(clampThresholdValue(127.6)).toBe(128)
    expect(clampThresholdValue(Number.NaN)).toBe(128)
  })

  it('normalizes threshold params without dropping invert', () => {
    expect(normalizeThresholdEffect({ threshold: Number.NaN, invert: true })).toEqual({
      threshold: 128,
      invert: true,
    })
  })

  it('creates a marker in percent plot coordinates with a call-time labelKey', () => {
    expect(createThresholdMarker({ threshold: 128 })).toEqual({
      id: RETOUCH_THRESHOLD_MARKER_ID,
      position: (128 / 255) * 100,
      value: 128,
      labelKey: 'canvas.retouch.thresholdMarkerAria',
    })
  })

  it('keeps the threshold marker locale-free with call-time t() copy parity', () => {
    // Marker carries only a labelKey (same precedent as definitions ranges /
    // S8 rail); the component translates at render time via
    // t(labelKey, {value}) so runtime locale switches stay reactive.
    const marker = createThresholdMarker({ threshold: 128 })
    expect(marker).not.toHaveProperty('ariaLabel')
    expect(marker).not.toHaveProperty('label')
    expect(marker.labelKey).toBe('canvas.retouch.thresholdMarkerAria')
    // Display copy parity: call-time t() still renders the legacy English copy.
    expect(i18n.global.t(marker.labelKey, { value: marker.value })).toBe('Threshold 128')
  })

  it('moves the threshold handle from percent plot coordinates and preserves invert', () => {
    expect(moveThresholdHandle({ threshold: 10, invert: true }, 50)).toEqual({
      threshold: 128,
      invert: true,
    })
    expect(moveThresholdHandle({ threshold: 10, invert: false }, Number.NaN)).toEqual({
      threshold: 128,
      invert: false,
    })
  })

  it('keeps threshold steps always effective with order-independent guards', () => {
    expect(isNeutralThresholdEffect({ threshold: 128, invert: false })).toBe(false)
    expect(isThresholdParams({ threshold: 10, invert: false })).toBe(true)
    // Sharpen shares the `threshold` key: the guard must not confuse it.
    expect(isThresholdParams({ amount: 10, radius: 1, threshold: 4, protectColor: true })).toBe(
      false,
    )
    expect(cloneThresholdParams({ threshold: 300, invert: true })).toEqual({
      threshold: 255,
      invert: true,
    })
    expect(cloneThresholdDefaults()).toEqual({ threshold: 128, invert: false })
  })
})
