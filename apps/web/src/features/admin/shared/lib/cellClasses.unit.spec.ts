import { describe, expect, it } from 'vitest'
import {
  adminInlineJumpClass,
  adminMonoClass,
  adminNumericCellClass,
  adminTruncateCellClass,
} from './cellClasses'

/**
 * Admin 单元格原子 class 契约：font-mono / 行内跳转 / 截断的单源。
 * 组件 spec 不能断言 class（见 checkTestGuardrails），所以几何契约只在这里断言一次。
 */
describe('cellClasses', () => {
  it('AdminMono 收口 font-mono', () => {
    expect(adminMonoClass).toContain('font-mono')
  })

  it('数值/日期列沿用 ledger/orders 范本：tabular-nums + whitespace-nowrap', () => {
    expect(adminNumericCellClass).toContain('tabular-nums')
    expect(adminNumericCellClass).toContain('whitespace-nowrap')
  })

  it('行内跳转以 orders 表单号 button 为范本：button + hover:text-primary + focus-visible:ring', () => {
    expect(adminInlineJumpClass).toContain('hover:text-primary')
    expect(adminInlineJumpClass).toContain('focus-visible:ring-2')
    expect(adminInlineJumpClass).toContain('font-mono')
    // 数值/单号同时满足 E6：等宽数字 + 不换行。
    expect(adminInlineJumpClass).toContain('tabular-nums')
    expect(adminInlineJumpClass).toContain('whitespace-nowrap')
  })

  it('截断规范是 truncate + title：class 只给 truncate，title 由调用方绑定', () => {
    expect(adminTruncateCellClass).toContain('truncate')
    expect(adminTruncateCellClass).not.toContain('line-clamp')
  })
})
