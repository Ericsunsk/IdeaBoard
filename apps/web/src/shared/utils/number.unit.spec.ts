import { describe, expect, it } from 'vitest'
import { clamp, clampUnit, clampWithFallback } from './number'

describe('clamp', () => {
  it('returns values inside the range unchanged', () => {
    expect(clamp(5, 0, 10)).toBe(5)
    expect(clamp(0, 0, 10)).toBe(0)
    expect(clamp(10, 0, 10)).toBe(10)
    expect(clamp(2.5, 0, 10)).toBe(2.5)
  })

  it('clamps values outside the range to the nearest bound', () => {
    expect(clamp(-3, 0, 10)).toBe(0)
    expect(clamp(42, 0, 10)).toBe(10)
    expect(clamp(-0.1, 0, 1)).toBe(0)
    expect(clamp(1.1, 0, 1)).toBe(1)
  })

  it('propagates non-finite input rather than substituting a default', () => {
    expect(clamp(Number.NaN, 0, 10)).toBeNaN()
    expect(clamp(Number.POSITIVE_INFINITY, 0, 10)).toBe(10)
    expect(clamp(Number.NEGATIVE_INFINITY, 0, 10)).toBe(0)
  })

  it('supports inverted-free use with negative bounds', () => {
    expect(clamp(-5, -10, -1)).toBe(-5)
    expect(clamp(-20, -10, -1)).toBe(-10)
    expect(clamp(3, -10, -1)).toBe(-1)
  })
})

describe('clampWithFallback', () => {
  it('behaves like clamp for finite values', () => {
    expect(clampWithFallback(5, 0, 10, 99)).toBe(5)
    expect(clampWithFallback(-3, 0, 10, 99)).toBe(0)
    expect(clampWithFallback(42, 0, 10, 99)).toBe(10)
  })

  it('resolves every non-finite value to the fallback', () => {
    expect(clampWithFallback(Number.NaN, 0, 10, 99)).toBe(99)
    expect(clampWithFallback(Number.POSITIVE_INFINITY, 0, 10, 99)).toBe(99)
    expect(clampWithFallback(Number.NEGATIVE_INFINITY, 0, 10, 99)).toBe(99)
  })

  it('returns the fallback as-is without clamping it', () => {
    expect(clampWithFallback(Number.NaN, 0, 10, -5)).toBe(-5)
    expect(clampWithFallback(Number.NaN, 0, 10, 500)).toBe(500)
  })
})

describe('clampUnit', () => {
  it('clamps into the unit range', () => {
    expect(clampUnit(0.5)).toBe(0.5)
    expect(clampUnit(-1)).toBe(0)
    expect(clampUnit(2)).toBe(1)
  })

  it('resolves non-finite values to 0 by default', () => {
    expect(clampUnit(Number.NaN)).toBe(0)
  })

  it('accepts a custom fallback for non-finite values', () => {
    expect(clampUnit(Number.NaN, 0.5)).toBe(0.5)
  })
})
