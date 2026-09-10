import { describe, expect, it } from 'vitest'
import {
  clampRetouchChannel,
  clampRetouchChannelRange,
  clampRetouchPlotPercent,
  clampRetouchRoundedPlotPercent,
  retouchChannelToPlotPercent,
  retouchPlotPercentToChannel,
} from './channelScale'

describe('retouchChannelScale', () => {
  it('clamps and rounds channel values with channel fallback semantics', () => {
    expect(clampRetouchChannel(-10)).toBe(0)
    expect(clampRetouchChannel(300)).toBe(255)
    expect(clampRetouchChannel(127.6)).toBe(128)
    expect(clampRetouchChannel(Number.NaN, 128)).toBe(128)
  })

  it('clamps channel values inside dynamic channel ranges', () => {
    expect(clampRetouchChannelRange(250, 0, 200, 128)).toBe(200)
    expect(clampRetouchChannelRange(12.4, 20, 80, 50)).toBe(20)
    expect(clampRetouchChannelRange(Number.POSITIVE_INFINITY, 20, 80, 50.6)).toBe(51)
  })

  it('converts channel values to continuous plot percent coordinates', () => {
    expect(retouchChannelToPlotPercent(0)).toBe(0)
    expect(retouchChannelToPlotPercent(127.5)).toBeCloseTo((128 / 255) * 100)
    expect(retouchChannelToPlotPercent(128)).toBeCloseTo((128 / 255) * 100)
    expect(retouchChannelToPlotPercent(255)).toBe(100)
  })

  it('converts plot percent coordinates back to rounded channel values', () => {
    expect(retouchPlotPercentToChannel(0)).toBe(0)
    expect(retouchPlotPercentToChannel(50)).toBe(128)
    expect(retouchPlotPercentToChannel(100)).toBe(255)
    expect(retouchPlotPercentToChannel(Number.NaN, 128)).toBe(128)
  })

  it('supports continuous and rounded plot percent clamping', () => {
    expect(clampRetouchPlotPercent(50.4)).toBe(50.4)
    expect(clampRetouchPlotPercent(-10)).toBe(0)
    expect(clampRetouchRoundedPlotPercent(50.4)).toBe(50)
    expect(clampRetouchRoundedPlotPercent(Number.NaN, 12.6)).toBe(13)
  })
})
