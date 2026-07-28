import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineAdminChartOptions } from './adminChart'

afterEach(() => {
  document.documentElement.style.removeProperty('--motion-enter-duration')
  vi.unstubAllGlobals()
})

describe('defineAdminChartOptions', () => {
  it('uses Motion Policy timing for every Admin chart', () => {
    document.documentElement.style.setProperty('--motion-enter-duration', '220ms')
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false })))

    expect(defineAdminChartOptions({ animation: { duration: 999 } }).animation).toEqual({
      duration: 220,
    })
  })

  it('keeps Admin chart data immediately readable under reduced motion', () => {
    document.documentElement.style.setProperty('--motion-enter-duration', '220ms')
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true })))

    expect(defineAdminChartOptions({}).animation).toBe(false)
  })
})
