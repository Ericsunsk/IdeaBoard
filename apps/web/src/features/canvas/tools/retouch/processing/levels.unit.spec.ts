import { describe, expect, it } from 'vitest'
import {
  clampLevelsHandles,
  createLevelsInputConstraints,
  createLevelsLut,
  createLevelsMarkers,
  createLevelsOutputConstraints,
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
    expect(moveLevelsHandle(lowEndpoint, 'midtones', 10).midtones).toBe(11)
    expect(moveLevelsHandle(highEndpoint, 'midtones', 20).midtones).toBe(19)
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
    expect(moveLevelsHandle(params, 'shadows', 250)).toEqual({
      shadows: 229,
      midtones: 229,
      highlights: 230,
      outputBlack: 0,
      outputWhite: 255,
    })
    expect(moveLevelsHandle(params, 'highlights', 5)).toEqual({
      shadows: 10,
      midtones: 11,
      highlights: 11,
      outputBlack: 0,
      outputWhite: 255,
    })
  })

  it('clamps the midtone handle between black and white points', () => {
    const params = { shadows: 51, midtones: 128, highlights: 204, outputBlack: 0, outputWhite: 255 }
    expect(moveLevelsHandle(params, 'midtones', 10)).toEqual({
      shadows: 51,
      midtones: 52,
      highlights: 204,
      outputBlack: 0,
      outputWhite: 255,
    })
    expect(moveLevelsHandle(params, 'midtones', 240)).toEqual({
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

  it('creates markers from clamped level handles', () => {
    expect(
      createLevelsMarkers({
        shadows: 252,
        midtones: Number.NaN,
        highlights: 51,
        outputBlack: 0,
        outputWhite: 255,
      }),
    ).toEqual([
      { id: 'shadows', position: 50, ariaLabel: 'Blacks 50', ariaValueMin: 0, ariaValueMax: 50 },
      {
        id: 'midtones',
        position: 51,
        ariaLabel: 'Midtones 51',
        ariaValueMin: 50,
        ariaValueMax: 51,
      },
      {
        id: 'highlights',
        position: 51,
        ariaLabel: 'Whites 51',
        ariaValueMin: 51,
        ariaValueMax: 255,
      },
    ])
  })

  it('moves output handles without allowing output levels to cross', () => {
    const params = { shadows: 0, midtones: 128, highlights: 255, outputBlack: 20, outputWhite: 220 }

    expect(moveLevelsOutputHandle(params, 'outputBlack', 240)).toEqual({
      shadows: 0,
      midtones: 128,
      highlights: 255,
      outputBlack: 219,
      outputWhite: 220,
    })
    expect(moveLevelsOutputHandle(params, 'outputWhite', 10)).toEqual({
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
})
