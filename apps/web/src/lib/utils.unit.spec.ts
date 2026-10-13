import { describe, expect, it } from 'vitest'
import { cn } from './utils'

describe('cn utility', () => {
  it('merges class names cleanly with clsx and tailwind-merge', () => {
    expect(cn('px-2 py-1', 'bg-red-500')).toBe('px-2 py-1 bg-red-500')
    expect(cn('px-2 py-1', 'px-4')).toBe('py-1 px-4')
    const falsyCondition = Boolean('')
    expect(cn('text-sm', falsyCondition && 'text-lg', null, undefined, 'font-bold')).toBe(
      'text-sm font-bold',
    )
  })

  it('merges canonical and domain border tokens deterministically', () => {
    expect(cn('border-page-chrome-border', 'border-destructive')).toBe('border-destructive')
    expect(cn('border-admin-border', 'border-page-chrome-border')).toBe('border-page-chrome-border')
    expect(cn('border-overlay-border', 'border-auth-input-border')).toBe('border-auth-input-border')
  })

  it('lets shared radius overrides replace base geometry', () => {
    expect(cn('rounded-control', 'rounded-pill')).toBe('rounded-pill')
    expect(cn('rounded-panel', 'rounded-overlay')).toBe('rounded-overlay')
    expect(cn('rounded-control', 'rounded-none')).toBe('rounded-none')
    expect(cn('rounded-overlay', 'rounded-t-control', 'rounded-t-panel')).toBe(
      'rounded-overlay rounded-t-panel',
    )
  })
})
