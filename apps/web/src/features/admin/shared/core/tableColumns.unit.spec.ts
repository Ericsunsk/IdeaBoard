import { describe, expect, it } from 'vitest'
import { ADMIN_TABLE_EXPANDER_COLUMN_WIDTH, ADMIN_TABLE_MORE_COLUMN_WIDTH } from './tableColumns'

/**
 * E6 列宽 token 契约：More 64px / expander 52px 全表统一。
 * 各表的硬编码 64px/52px 必须引用这两个常量，值本身只在这里断言一次。
 */
describe('tableColumns', () => {
  it('固化 More 列宽为 64px', () => {
    expect(ADMIN_TABLE_MORE_COLUMN_WIDTH).toBe('64px')
  })

  it('固化 expander 列宽为 52px', () => {
    expect(ADMIN_TABLE_EXPANDER_COLUMN_WIDTH).toBe('52px')
  })
})
