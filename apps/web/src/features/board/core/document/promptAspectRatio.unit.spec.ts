import { describe, expect, it } from 'vitest'
import {
  normalizePromptAspectRatio,
  resolvePromptAspectRatioValue,
} from './promptAspectRatio'

describe('promptAspectRatio', () => {
  it('normalizes invalid or unknown aspect ratio to default auto', () => {
    expect(normalizePromptAspectRatio(null)).toBe('auto')
    expect(normalizePromptAspectRatio(undefined)).toBe('auto')
    expect(normalizePromptAspectRatio('invalid')).toBe('auto')
    expect(normalizePromptAspectRatio('4:3')).toBe('4:3')
  })

  it('resolves ratio values using mathematical formula h/w', () => {
    expect(resolvePromptAspectRatioValue('1:1')).toBe(1)
    expect(resolvePromptAspectRatioValue('3:4')).toBe(4 / 3)
    expect(resolvePromptAspectRatioValue('4:3')).toBe(3 / 4)
    expect(resolvePromptAspectRatioValue('9:16')).toBe(16 / 9)
    expect(resolvePromptAspectRatioValue('16:9')).toBe(9 / 16)
    expect(resolvePromptAspectRatioValue('2:3')).toBe(3 / 2)
  })

  it('returns null for non-ratio strings or auto', () => {
    expect(resolvePromptAspectRatioValue('auto')).toBeNull()
    expect(resolvePromptAspectRatioValue('')).toBeNull()
    expect(resolvePromptAspectRatioValue('invalid')).toBeNull()
    expect(resolvePromptAspectRatioValue('0:10')).toBeNull()
    expect(resolvePromptAspectRatioValue('-1:1')).toBeNull()
  })
})
