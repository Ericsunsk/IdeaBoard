import { describe, expect, it } from 'vitest'
import {
  clampImageEditorChannel,
  clampImageEditorChannelRange,
  clampImageEditorPlotPercent,
  clampImageEditorRoundedPlotPercent,
  imageEditorChannelToPlotPercent,
  imageEditorPlotPercentToChannel,
} from './imageEditorChannelScale'

describe('imageEditorChannelScale', () => {
  it('clamps and rounds channel values with channel fallback semantics', () => {
    expect(clampImageEditorChannel(-10)).toBe(0)
    expect(clampImageEditorChannel(300)).toBe(255)
    expect(clampImageEditorChannel(127.6)).toBe(128)
    expect(clampImageEditorChannel(Number.NaN, 128)).toBe(128)
  })

  it('clamps channel values inside dynamic channel ranges', () => {
    expect(clampImageEditorChannelRange(250, 0, 200, 128)).toBe(200)
    expect(clampImageEditorChannelRange(12.4, 20, 80, 50)).toBe(20)
    expect(clampImageEditorChannelRange(Number.POSITIVE_INFINITY, 20, 80, 50.6)).toBe(51)
  })

  it('converts channel values to continuous plot percent coordinates', () => {
    expect(imageEditorChannelToPlotPercent(0)).toBe(0)
    expect(imageEditorChannelToPlotPercent(127.5)).toBeCloseTo((128 / 255) * 100)
    expect(imageEditorChannelToPlotPercent(128)).toBeCloseTo((128 / 255) * 100)
    expect(imageEditorChannelToPlotPercent(255)).toBe(100)
  })

  it('converts plot percent coordinates back to rounded channel values', () => {
    expect(imageEditorPlotPercentToChannel(0)).toBe(0)
    expect(imageEditorPlotPercentToChannel(50)).toBe(128)
    expect(imageEditorPlotPercentToChannel(100)).toBe(255)
    expect(imageEditorPlotPercentToChannel(Number.NaN, 128)).toBe(128)
  })

  it('supports continuous and rounded plot percent clamping', () => {
    expect(clampImageEditorPlotPercent(50.4)).toBe(50.4)
    expect(clampImageEditorPlotPercent(-10)).toBe(0)
    expect(clampImageEditorRoundedPlotPercent(50.4)).toBe(50)
    expect(clampImageEditorRoundedPlotPercent(Number.NaN, 12.6)).toBe(13)
  })
})
