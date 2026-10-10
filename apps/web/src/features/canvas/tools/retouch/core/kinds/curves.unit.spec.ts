import { describe, expect, it } from 'vitest'
import { i18n } from '../../../../../../core/i18n'
import type { RetouchCurvesParams } from '../model.ts'
import {
  RETOUCH_CURVE_ENDPOINT_BLACK_ID,
  RETOUCH_CURVE_ENDPOINT_WHITE_ID,
  RETOUCH_CURVE_POINT_PREFIX,
  RETOUCH_CURVE_PRESET_PREFIX,
  RETOUCH_CURVE_PRESETS,
  addCurvePoint,
  applyCurvePreset,
  createCurvesLuts,
  createIdentityCurvePoints,
  denormalizeCurvesParams,
  isCurvesParams,
  isNeutralCurvesEffect,
  cloneCurvesDefaults,
  cloneCurvesParams,
  moveCurvePoint,
  normalizeCurveChannelPoints,
  normalizeCurvesParams,
  removeCurvePoint,
  resolveCurvePresetId,
  sampleCurvePointsPath,
} from './curves'

const EXPECTED_MAX_USER_POINTS = 16

function lutValues(lut: Uint8ClampedArray, values: readonly number[]) {
  return values.map((value) => lut[value])
}

function createRgbCurveLut(points: RetouchCurvesParams['channels']['rgb']) {
  return createCurvesLuts(
    normalizeCurvesParams({ channels: points === undefined ? {} : { rgb: points } }),
  ).red
}

describe('retouchCurves', () => {
  it('normalizes identity curves to locked endpoints', () => {
    const normalized = normalizeCurveChannelPoints([])

    expect(normalized).toEqual([
      {
        id: RETOUCH_CURVE_ENDPOINT_BLACK_ID,
        input: 0,
        output: 0,
        inputChannel: 0,
        outputChannel: 0,
        endpoint: true,
      },
      {
        id: RETOUCH_CURVE_ENDPOINT_WHITE_ID,
        input: 100,
        output: 100,
        inputChannel: 255,
        outputChannel: 255,
        endpoint: true,
      },
    ])
    expect(normalizeCurvesParams({ channels: {} })).toEqual({ channels: {} })
  })

  it('falls back malformed current-shape params to neutral curves', () => {
    const malformedParams = {
      channels: {
        rgb: [null, false, { id: 'valid', input: 40, output: 45 }],
        red: null,
      },
    } as unknown as RetouchCurvesParams

    expect(normalizeCurvesParams(malformedParams).channels.rgb).toContainEqual(
      expect.objectContaining({
        id: 'valid',
        input: 40,
        output: 45,
        endpoint: false,
      }),
    )
    expect(
      normalizeCurvesParams({
        channels: { rgb: true } as unknown as RetouchCurvesParams['channels'],
      }),
    ).toEqual({ channels: {} })
  })

  it('normalizes endpoints, out-of-range values, duplicate inputs, and blank ids', () => {
    const normalized = normalizeCurveChannelPoints([
      { id: 'black', input: -10, output: 12.2 },
      { id: 'white', input: 150, output: 88.7 },
      { id: 'first', input: 25.2, output: -20 },
      { id: 'replacement', input: 25.4, output: 60 },
      { id: '   ', input: 50.1, output: Number.POSITIVE_INFINITY },
    ])

    expect(
      normalized.map(({ id, input, output, endpoint }) => ({ id, input, output, endpoint })),
    ).toEqual([
      { id: RETOUCH_CURVE_ENDPOINT_BLACK_ID, input: 0, output: 12, endpoint: true },
      { id: 'replacement', input: 25, output: 60, endpoint: false },
      { id: 'point:50', input: 50, output: 50, endpoint: false },
      { id: RETOUCH_CURVE_ENDPOINT_WHITE_ID, input: 100, output: 89, endpoint: true },
    ])
  })

  it('normalizes user points away from reserved endpoint ids', () => {
    const normalized = normalizeCurveChannelPoints([
      { id: RETOUCH_CURVE_ENDPOINT_BLACK_ID, input: 40, output: 45 },
      { id: RETOUCH_CURVE_ENDPOINT_WHITE_ID, input: 60, output: 65 },
    ])

    expect(normalized).toContainEqual(
      expect.objectContaining({
        id: 'point:40',
        input: 40,
        output: 45,
        endpoint: false,
      }),
    )
    expect(normalized).toContainEqual(
      expect.objectContaining({
        id: 'point:60',
        input: 60,
        output: 65,
        endpoint: false,
      }),
    )
  })

  it('normalizes missing and non-string point ids to stable input-derived ids', () => {
    const normalized = normalizeCurveChannelPoints([
      { input: 35, output: 40 },
      { id: 123, input: 65, output: 70 },
    ] as unknown as Parameters<typeof normalizeCurveChannelPoints>[0])

    expect(normalized).toContainEqual(
      expect.objectContaining({
        id: 'point:35',
        input: 35,
        output: 40,
        endpoint: false,
      }),
    )
    expect(normalized).toContainEqual(
      expect.objectContaining({
        id: 'point:65',
        input: 65,
        output: 70,
        endpoint: false,
      }),
    )
  })

  it('falls back invalid point and endpoint outputs to their neutral input output', () => {
    const normalized = normalizeCurveChannelPoints([
      { id: 'black', input: 0, output: Number.NaN },
      { id: 'mid', input: 42, output: Number.POSITIVE_INFINITY },
      { id: 'white', input: 100, output: 'invalid' as unknown as number },
    ])

    expect(normalized.map(({ input, output, endpoint }) => ({ input, output, endpoint }))).toEqual([
      { input: 0, output: 0, endpoint: true },
      { input: 42, output: 42, endpoint: false },
      { input: 100, output: 100, endpoint: true },
    ])
  })

  it('keeps only the maximum supported dense user points', () => {
    const densePoints = Array.from({ length: EXPECTED_MAX_USER_POINTS + 4 }, (_, index) => ({
      id: `point:${index + 1}`,
      input: index + 1,
      output: index + 1,
    }))

    const normalized = normalizeCurveChannelPoints(densePoints)
    const userPoints = normalized.filter((point) => !point.endpoint)

    expect(userPoints).toHaveLength(EXPECTED_MAX_USER_POINTS)
    expect(userPoints[0]?.input).toBe(1)
    expect(userPoints.at(-1)?.input).toBe(EXPECTED_MAX_USER_POINTS)
  })

  it('creates identity, brighten, darken, S curve, and endpoint LUTs', () => {
    const identity = createCurvesLuts(normalizeCurvesParams({ channels: {} })).red
    const brighten = createRgbCurveLut([{ id: 'mid', input: 50, output: 63 }])
    const darken = createRgbCurveLut([{ id: 'mid', input: 50, output: 38 }])
    const sCurve = createRgbCurveLut([
      { id: 'shadow', input: 25, output: 15 },
      { id: 'highlight', input: 75, output: 85 },
    ])
    const liftedBlack = createRgbCurveLut([
      { id: RETOUCH_CURVE_ENDPOINT_BLACK_ID, input: 0, output: 20 },
    ])
    const loweredWhite = createRgbCurveLut([
      { id: RETOUCH_CURVE_ENDPOINT_WHITE_ID, input: 100, output: 80 },
    ])

    expect(lutValues(identity, [0, 64, 128, 192, 255])).toEqual([0, 64, 128, 192, 255])
    expect(brighten[128]).toBeGreaterThan(128)
    expect(darken[128]).toBeLessThan(128)
    expect(sCurve[64]).toBeLessThan(64)
    expect(sCurve[192]).toBeGreaterThan(192)
    expect(liftedBlack[0]).toBe(51)
    expect(loweredWhite[255]).toBe(204)
  })

  it('composes RGB curves before single-channel curves', () => {
    const luts = createCurvesLuts(
      normalizeCurvesParams({
        channels: {
          rgb: [{ id: 'rgb', input: 50, output: 63 }],
          red: [{ id: 'red', input: 63, output: 50 }],
          green: [{ id: 'green', input: 63, output: 38 }],
        },
      }),
    )

    expect(luts.red[128]).toBe(128)
    expect(luts.green[128]).toBeLessThan(128)
    expect(luts.blue[128]).toBeGreaterThan(128)
  })

  it('creates display Bezier paths from the float curve transfer instead of quantized LUT values', () => {
    const path = sampleCurvePointsPath(createIdentityCurvePoints())

    expect(path).toBe('M 0 100 C 33.333 66.667 66.667 33.333 100 0')
    expect(path).not.toContain('49.804')
    expect(path).toContain(' C ')
  })

  it('applies presets and resolves custom params', () => {
    const brighten = applyCurvePreset('brighten')
    const unknown = applyCurvePreset('missing')

    expect(brighten.channels.rgb?.[0]?.id.startsWith(RETOUCH_CURVE_PRESET_PREFIX)).toBe(true)
    expect(resolveCurvePresetId({ channels: {} })).toBe('linear')
    expect(resolveCurvePresetId(brighten)).toBe('brighten')
    expect(resolveCurvePresetId(unknown)).toBe('linear')
    expect(
      resolveCurvePresetId({ channels: { red: [{ id: 'custom', input: 50, output: 40 }] } }),
    ).toBe('custom')
  })

  it('exposes preset labelKeys for call-time t() without baking locale copy', () => {
    // Presets carry only labelKeys (same precedent as definitions ranges /
    // S8 rail); CurvesControl translates at render time via t(labelKey) so
    // runtime locale switches stay reactive. Nothing is baked at module load.
    expect(RETOUCH_CURVE_PRESETS.map((preset) => [preset.id, preset.labelKey])).toEqual([
      ['linear', 'canvas.retouch.curvesPresets.linear'],
      ['brighten', 'canvas.retouch.curvesPresets.brighten'],
      ['darken', 'canvas.retouch.curvesPresets.darken'],
      ['soft-contrast', 'canvas.retouch.curvesPresets.softContrast'],
      ['strong-contrast', 'canvas.retouch.curvesPresets.strongContrast'],
      ['matte', 'canvas.retouch.curvesPresets.matte'],
    ])
    for (const preset of RETOUCH_CURVE_PRESETS) {
      expect(preset).not.toHaveProperty('label')
    }
    // Display copy parity: call-time t() still renders the legacy English copy.
    expect(
      Object.fromEntries(
        RETOUCH_CURVE_PRESETS.map((preset) => [preset.id, i18n.global.t(preset.labelKey)]),
      ),
    ).toEqual({
      linear: 'Linear',
      brighten: 'Brighten',
      darken: 'Darken',
      'soft-contrast': 'Soft contrast',
      'strong-contrast': 'Strong contrast',
      matte: 'Matte',
    })
  })

  it('adds, moves, and removes points while locking endpoints', () => {
    const params: RetouchCurvesParams = {
      channels: {
        rgb: [
          { id: 'shadow', input: 25, output: 25 },
          { id: 'highlight', input: 75, output: 75 },
        ],
      },
    }

    const added = addCurvePoint(params, 'rgb', { id: 'mid', input: 50, output: 60 })
    expect(added.channels.rgb).toEqual([
      { id: 'shadow', input: 25, output: 25 },
      { id: 'mid', input: 50, output: 60 },
      { id: 'highlight', input: 75, output: 75 },
    ])

    const moved = moveCurvePoint(added, 'rgb', 'mid', { input: 90, output: 120 })
    expect(moved.channels.rgb).toContainEqual({ id: 'mid', input: 74, output: 100 })

    const movedBlackEndpoint = moveCurvePoint(moved, 'rgb', RETOUCH_CURVE_ENDPOINT_BLACK_ID, {
      input: 40,
      output: 20,
    })
    expect(movedBlackEndpoint.channels.rgb).toContainEqual({
      id: RETOUCH_CURVE_ENDPOINT_BLACK_ID,
      input: 0,
      output: 20,
    })

    expect(removeCurvePoint(movedBlackEndpoint, 'rgb', RETOUCH_CURVE_ENDPOINT_BLACK_ID)).toBe(
      movedBlackEndpoint,
    )
    expect(removeCurvePoint(movedBlackEndpoint, 'rgb', 'mid').channels.rgb).toEqual([
      { id: RETOUCH_CURVE_ENDPOINT_BLACK_ID, input: 0, output: 20 },
      { id: 'shadow', input: 25, output: 25 },
      { id: 'highlight', input: 75, output: 75 },
    ])
  })

  it('generates random curve point ids under the shared point prefix', () => {
    const added = addCurvePoint({ channels: {} }, 'rgb', { input: 50, output: 60 })
    const pointId = added.channels.rgb?.[0]?.id ?? ''
    expect(pointId.startsWith(RETOUCH_CURVE_POINT_PREFIX)).toBe(true)
  })

  it('denormalizes shared channel state by dropping identity endpoints', () => {
    const denormalized = denormalizeCurvesParams(
      normalizeCurvesParams({
        channels: {
          rgb: [
            { id: 'shadow', input: 25, output: 25 },
            { id: 'mid', input: 50, output: 60 },
          ],
        },
      }),
    )

    expect(denormalized).toEqual({
      channels: {
        rgb: [
          { id: 'shadow', input: 25, output: 25 },
          { id: 'mid', input: 50, output: 60 },
        ],
      },
    })
    expect(denormalizeCurvesParams(normalizeCurvesParams({ channels: {} }))).toEqual({
      channels: {},
    })
  })

  it('owns its trait contract: neutral check, guard, and sanitizing clone', () => {
    expect(isNeutralCurvesEffect({ channels: {} })).toBe(true)
    expect(
      isNeutralCurvesEffect(
        normalizeCurvesParams({ channels: { rgb: [{ id: 'mid', input: 50, output: 60 }] } }),
      ),
    ).toBe(false)
    expect(isCurvesParams({ channels: {} })).toBe(true)
    expect(isCurvesParams({ threshold: 1, invert: true })).toBe(false)
    const source: RetouchCurvesParams = {
      channels: { rgb: [{ id: 'mid', input: 50, output: 60 }] },
    }
    const cloned = cloneCurvesParams(source)
    expect(cloned).toEqual(source)
    expect(cloned).not.toBe(source)
    expect(cloned.channels).not.toBe(source.channels)
    expect(cloneCurvesDefaults()).toEqual({ channels: {} })
  })
})
