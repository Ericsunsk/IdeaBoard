import { describe, expect, it } from 'vitest'
import { ADMIN_IDENTITY_EMPTY, ADMIN_NUMERIC_EMPTY, emptyPlaceholder } from './cellPlaceholders'

/**
 * Admin 单元格空态占位契约：数字表用 —，标识类用 -。
 * pricing 的「未设置 / —」是档位语义，不在此覆盖。
 */
describe('cellPlaceholders', () => {
  it('终结数字表的空态占位为 —', () => {
    expect(ADMIN_NUMERIC_EMPTY).toBe('—')
    expect(emptyPlaceholder(null, 'numeric')).toBe('—')
    expect(emptyPlaceholder(undefined, 'numeric')).toBe('—')
    expect(emptyPlaceholder('', 'numeric')).toBe('—')
  })

  it('终结标识类的空态占位为 -', () => {
    expect(ADMIN_IDENTITY_EMPTY).toBe('-')
    expect(emptyPlaceholder(null)).toBe('-')
    expect(emptyPlaceholder(undefined)).toBe('-')
    expect(emptyPlaceholder('', 'identity')).toBe('-')
  })

  it('数字 0 是有效值，不坍缩成占位符', () => {
    expect(emptyPlaceholder(0, 'numeric')).toBe('0')
    expect(emptyPlaceholder(0)).toBe('0')
  })

  it('非空值原样转字符串', () => {
    expect(emptyPlaceholder('a@b.c')).toBe('a@b.c')
    expect(emptyPlaceholder(12, 'numeric')).toBe('12')
  })
})
