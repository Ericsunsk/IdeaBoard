import { describe, expect, it } from 'vitest'
import { normalizePageSize, PAGE_SIZES } from './pagination'

describe('PAGE_SIZES', () => {
  it('keeps the canonical per-surface page sizes', () => {
    expect(PAGE_SIZES.standard).toBe(20)
    expect(PAGE_SIZES.adminLedger).toBe(25)
    expect(PAGE_SIZES.adminFindingQueue).toBe(25)
    expect(PAGE_SIZES.adminUsers).toBe(10)
    expect(PAGE_SIZES.adminMonitor).toBe(15)
    expect(PAGE_SIZES.dense).toBe(100)
    expect(PAGE_SIZES.max).toBe(100)
  })
})

describe('normalizePageSize', () => {
  it('falls back to the standard default for missing or non-numeric input', () => {
    expect(normalizePageSize(undefined)).toBe(PAGE_SIZES.standard)
    expect(normalizePageSize(Number.NaN)).toBe(PAGE_SIZES.standard)
    expect(normalizePageSize(Number.POSITIVE_INFINITY)).toBe(PAGE_SIZES.standard)
    expect(normalizePageSize('20')).toBe(PAGE_SIZES.standard)
    expect(normalizePageSize(null)).toBe(PAGE_SIZES.standard)
  })

  it('floors fractional values, floors at 1, and caps at the max', () => {
    expect(normalizePageSize(20.9)).toBe(20)
    expect(normalizePageSize(0)).toBe(1)
    expect(normalizePageSize(-3)).toBe(1)
    expect(normalizePageSize(999)).toBe(PAGE_SIZES.max)
  })

  it('honors per-surface defaults and ceilings', () => {
    expect(normalizePageSize(undefined, PAGE_SIZES.adminUsers)).toBe(10)
    expect(normalizePageSize(Number.NaN, PAGE_SIZES.adminUsers)).toBe(10)
    expect(normalizePageSize(999, PAGE_SIZES.adminUsers)).toBe(PAGE_SIZES.max)
    expect(normalizePageSize(0, PAGE_SIZES.adminUsers)).toBe(1)
    expect(normalizePageSize(undefined, PAGE_SIZES.adminMonitor)).toBe(15)
    expect(normalizePageSize(3.7, 1, Number.POSITIVE_INFINITY)).toBe(3)
    expect(normalizePageSize(Number.NaN, 1, Number.POSITIVE_INFINITY)).toBe(1)
  })
})
