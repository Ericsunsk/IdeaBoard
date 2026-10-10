import { describe, expect, it } from 'vitest'
import { i18n } from '../../../../../../shared/i18n'
import {
  RETOUCH_LEVELS_HIGHLIGHTS_MARKER_ID,
  RETOUCH_LEVELS_MIDTONES_MARKER_ID,
  RETOUCH_LEVELS_OUTPUT_BLACK_MARKER_ID,
  RETOUCH_LEVELS_OUTPUT_WHITE_MARKER_ID,
  RETOUCH_LEVELS_SHADOWS_MARKER_ID,
  clampLevelsHandles,
  cloneLevelsDefaults,
  cloneLevelsParams,
  createLevelsInputConstraints,
  createLevelsLut,
  createLevelsMarkers,
  createLevelsOutputConstraints,
  isLevelsParams,
  isNeutralLevelsEffect,
  moveLevelsHandle,
  moveLevelsOutputHandle,
  normalizeLevelsEffect,
} from './levels'

describe('retouchLevels', () => {
  it('normalizes integer channel handles without changing neutral midtones', () => {
    expect(
      normalizeLevelsEffect({
        shadows: 64,
        midtones: 128,
        highlights: 191,
        outputBlack: 0,
        outputWhite: 255,
      }),
    ).toEqual({
      blackPoint: 64,
      whitePoint: 191,
      gamma: 1,
      outputBlack: 0,
      outputWhite: 255,
    })
  })

  it('keeps normalized gamma finite for boundary and non-finite midtones', () => {
    expect(
      Number.isFinite(
        normalizeLevelsEffect({
          shadows: 0,
          midtones: 260,
          highlights: 255,
          outputBlack: 0,
          outputWhite: 255,
        }).gamma,
      ),
    ).toBe(true)
    expect(
      normalizeLevelsEffect({
        shadows: 0,
        midtones: Number.NaN,
        highlights: 255,
        outputBlack: 0,
        outputWhite: 255,
      }).gamma,
    ).toBe(1)
  })

  it('keeps midtones inside black and white points when there is space', () => {
    const lowEndpoint = {
      shadows: 10,
      midtones: 10,
      highlights: 20,
      outputBlack: 0,
      outputWhite: 255,
    }
    const highEndpoint = {
      shadows: 10,
      midtones: 20,
      highlights: 20,
      outputBlack: 0,
      outputWhite: 255,
    }

    expect(clampLevelsHandles(lowEndpoint).midtones).toBe(11)
    expect(clampLevelsHandles(highEndpoint).midtones).toBe(19)
    expect(moveLevelsHandle(lowEndpoint, RETOUCH_LEVELS_MIDTONES_MARKER_ID, 10).midtones).toBe(11)
    expect(moveLevelsHandle(highEndpoint, RETOUCH_LEVELS_MIDTONES_MARKER_ID, 20).midtones).toBe(19)
    expect(normalizeLevelsEffect(lowEndpoint).gamma).toBeCloseTo(Math.log(0.5) / Math.log(0.1))
    expect(normalizeLevelsEffect(lowEndpoint).gamma).not.toBe(1)
  })

  it('allows midtones to degrade to endpoints only when no interior channel exists', () => {
    const params = {
      shadows: 50,
      midtones: 50,
      highlights: 51,
      outputBlack: 0,
      outputWhite: 255,
    }

    expect(clampLevelsHandles(params)).toEqual(params)
    expect(createLevelsInputConstraints(params).midtones).toEqual({ min: 50, max: 51 })
    expect(normalizeLevelsEffect(params).gamma).toBe(1)
  })

  it('normalizes invalid handle ordering through the same clamp rules as the UI', () => {
    const normalized = normalizeLevelsEffect({
      shadows: 252,
      midtones: 128,
      highlights: 51,
      outputBlack: 0,
      outputWhite: 255,
    })

    expect(normalized.blackPoint).toBe(50)
    expect(normalized.whitePoint).toBe(51)
    expect(Number.isFinite(normalized.gamma)).toBe(true)
  })

  it('clamps black and white handles with a minimum spacing', () => {
    expect(
      clampLevelsHandles({
        shadows: -10,
        midtones: 128,
        highlights: 300,
        outputBlack: -10,
        outputWhite: 300,
      }),
    ).toEqual({
      shadows: 0,
      midtones: 128,
      highlights: 255,
      outputBlack: 0,
      outputWhite: 255,
    })
    expect(
      clampLevelsHandles({
        shadows: 252,
        midtones: 128,
        highlights: 51,
        outputBlack: 254,
        outputWhite: 10,
      }),
    ).toEqual({
      shadows: 50,
      midtones: 51,
      highlights: 51,
      outputBlack: 254,
      outputWhite: 255,
    })
  })

  it('moves handles without allowing black and white crossing', () => {
    const params = { shadows: 10, midtones: 128, highlights: 230, outputBlack: 0, outputWhite: 255 }
    expect(moveLevelsHandle(params, RETOUCH_LEVELS_SHADOWS_MARKER_ID, 250)).toEqual({
      shadows: 229,
      midtones: 229,
      highlights: 230,
      outputBlack: 0,
      outputWhite: 255,
    })
    expect(moveLevelsHandle(params, RETOUCH_LEVELS_HIGHLIGHTS_MARKER_ID, 5)).toEqual({
      shadows: 10,
      midtones: 11,
      highlights: 11,
      outputBlack: 0,
      outputWhite: 255,
    })
  })

  it('clamps the midtone handle between black and white points', () => {
    const params = { shadows: 51, midtones: 128, highlights: 204, outputBlack: 0, outputWhite: 255 }
    expect(moveLevelsHandle(params, RETOUCH_LEVELS_MIDTONES_MARKER_ID, 10)).toEqual({
      shadows: 51,
      midtones: 52,
      highlights: 204,
      outputBlack: 0,
      outputWhite: 255,
    })
    expect(moveLevelsHandle(params, RETOUCH_LEVELS_MIDTONES_MARKER_ID, 240)).toEqual({
      shadows: 51,
      midtones: 203,
      highlights: 204,
      outputBlack: 0,
      outputWhite: 255,
    })
  })

  it('creates dynamic channel constraints for level and output handles', () => {
    expect(
      createLevelsInputConstraints({
        shadows: 64,
        midtones: 128,
        highlights: 191,
        outputBlack: 20,
        outputWhite: 220,
      }),
    ).toEqual({
      shadows: { min: 0, max: 190 },
      midtones: { min: 65, max: 190 },
      highlights: { min: 65, max: 255 },
    })
    expect(
      createLevelsOutputConstraints({
        shadows: 64,
        midtones: 128,
        highlights: 191,
        outputBlack: 20,
        outputWhite: 220,
      }),
    ).toEqual({
      outputBlack: { min: 0, max: 219 },
      outputWhite: { min: 21, max: 255 },
    })
  })

  it('creates markers from clamped level handles with call-time labelKeys', () => {
    expect(
      createLevelsMarkers({
        shadows: 252,
        midtones: Number.NaN,
        highlights: 51,
        outputBlack: 0,
        outputWhite: 255,
      }),
    ).toEqual([
      {
        id: RETOUCH_LEVELS_SHADOWS_MARKER_ID,
        position: 50,
        labelKey: 'canvas.retouch.shadowsMarkerAria',
        ariaValueMin: 0,
        ariaValueMax: 50,
      },
      {
        id: RETOUCH_LEVELS_MIDTONES_MARKER_ID,
        position: 51,
        labelKey: 'canvas.retouch.midtonesMarkerAria',
        ariaValueMin: 50,
        ariaValueMax: 51,
      },
      {
        id: RETOUCH_LEVELS_HIGHLIGHTS_MARKER_ID,
        position: 51,
        labelKey: 'canvas.retouch.highlightsMarkerAria',
        ariaValueMin: 51,
        ariaValueMax: 255,
      },
    ])
  })

  it('keeps level markers locale-free with call-time t() copy parity', () => {
    // Markers carry only labelKeys (same precedent as definitions ranges /
    // S8 rail); components translate at render time via t(labelKey, {value})
    // so runtime locale switches stay reactive. Nothing is baked here.
    const markers = createLevelsMarkers({
      shadows: 50,
      midtones: 51,
      highlights: 51,
      outputBlack: 0,
      outputWhite: 255,
    })
    for (const marker of markers) {
      expect(marker).not.toHaveProperty('ariaLabel')
      expect(marker).not.toHaveProperty('label')
    }
    expect(markers.map((marker) => marker.labelKey)).toEqual([
      'canvas.retouch.shadowsMarkerAria',
      'canvas.retouch.midtonesMarkerAria',
      'canvas.retouch.highlightsMarkerAria',
    ])
    // Display copy parity: call-time t() still renders the legacy English copy.
    expect(i18n.global.t(markers[0]?.labelKey as string, { value: markers[0]?.position })).toBe(
      'Blacks 50',
    )
    expect(i18n.global.t(markers[1]?.labelKey as string, { value: markers[1]?.position })).toBe(
      'Midtones 51',
    )
    expect(i18n.global.t(markers[2]?.labelKey as string, { value: markers[2]?.position })).toBe(
      'Whites 51',
    )
  })

  it('moves output handles without allowing output levels to cross', () => {
    const params = { shadows: 0, midtones: 128, highlights: 255, outputBlack: 20, outputWhite: 220 }

    expect(moveLevelsOutputHandle(params, RETOUCH_LEVELS_OUTPUT_BLACK_MARKER_ID, 240)).toEqual({
      shadows: 0,
      midtones: 128,
      highlights: 255,
      outputBlack: 219,
      outputWhite: 220,
    })
    expect(moveLevelsOutputHandle(params, RETOUCH_LEVELS_OUTPUT_WHITE_MARKER_ID, 10)).toEqual({
      shadows: 0,
      midtones: 128,
      highlights: 255,
      outputBlack: 20,
      outputWhite: 21,
    })
  })

  it('creates the same LUT behavior as the existing levels retouch path', () => {
    const lut = createLevelsLut({
      blackPoint: 64,
      whitePoint: 191,
      gamma: 1,
      outputBlack: 0,
      outputWhite: 255,
    })

    expect(lut[0]).toBe(0)
    expect(lut[64]).toBe(0)
    expect(lut[128]).toBe(129)
    expect(lut[191]).toBe(255)
    expect(lut[255]).toBe(255)
  })

  it('maps normalized levels into the configured output range', () => {
    const lut = createLevelsLut({
      blackPoint: 0,
      whitePoint: 255,
      gamma: 1,
      outputBlack: 32,
      outputWhite: 224,
    })

    expect(lut[0]).toBe(32)
    expect(lut[128]).toBe(128)
    expect(lut[255]).toBe(224)
  })

  it('owns its trait contract: neutral check, guard, and sanitizing clone', () => {
    expect(
      isNeutralLevelsEffect({
        blackPoint: 0,
        whitePoint: 255,
        gamma: 1,
        outputBlack: 0,
        outputWhite: 255,
      }),
    ).toBe(true)
    expect(
      isNeutralLevelsEffect({
        blackPoint: 64,
        whitePoint: 191,
        gamma: 1,
        outputBlack: 0,
        outputWhite: 255,
      }),
    ).toBe(false)
    expect(
      isLevelsParams({
        shadows: 0,
        midtones: 128,
        highlights: 255,
        outputBlack: 0,
        outputWhite: 255,
      }),
    ).toBe(true)
    expect(isLevelsParams({ channels: {} })).toBe(false)
    expect(
      cloneLevelsParams({
        shadows: -10,
        midtones: 128,
        highlights: 300,
        outputBlack: 0,
        outputWhite: 255,
      }),
    ).toEqual({ shadows: 0, midtones: 128, highlights: 255, outputBlack: 0, outputWhite: 255 })
    expect(cloneLevelsDefaults()).toEqual({
      shadows: 0,
      midtones: 128,
      highlights: 255,
      outputBlack: 0,
      outputWhite: 255,
    })
  })
})
