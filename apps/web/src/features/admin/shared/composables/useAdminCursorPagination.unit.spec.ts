import { ref } from 'vue'
import { describe, expect, it } from 'vitest'
import { useAdminCursorPagination } from './useAdminCursorPagination'

describe('useAdminCursorPagination', () => {
  it('advances to a discovered cursor and retreats to the previous page', () => {
    const pagination = useAdminCursorPagination<number>()

    expect(pagination.cursor.value).toBeUndefined()
    expect(pagination.currentPage.value).toBe(1)

    pagination.setNextCursor(42)
    expect(pagination.advance()).toBe(true)
    expect(pagination.cursor.value).toBe(42)
    expect(pagination.currentPage.value).toBe(2)

    expect(pagination.retreat()).toBe(true)
    expect(pagination.cursor.value).toBeUndefined()
    expect(pagination.currentPage.value).toBe(1)
  })

  it('resets the current cursor and page history to the initial page', () => {
    const pagination = useAdminCursorPagination<string>()

    pagination.setNextCursor('page-2')
    pagination.advance()
    pagination.setNextCursor('page-3')
    pagination.advance()

    pagination.reset()

    expect(pagination.cursor.value).toBeUndefined()
    expect(pagination.currentPage.value).toBe(1)
    expect(pagination.retreat()).toBe(false)
  })

  it('blocks forward and backward navigation while a page is pending', () => {
    const pending = ref(true)
    const pagination = useAdminCursorPagination<number>({ pending })

    pagination.setNextCursor(2)
    expect(pagination.canAdvance.value).toBe(false)
    expect(pagination.advance()).toBe(false)

    pending.value = false
    expect(pagination.advance()).toBe(true)

    pending.value = true
    expect(pagination.canRetreat.value).toBe(false)
    expect(pagination.retreat()).toBe(false)
    expect(pagination.currentPage.value).toBe(2)
  })

  it('exposes the known terminal page without inventing a next page', () => {
    const pagination = useAdminCursorPagination<number>()

    expect(pagination.hasNext.value).toBe(false)
    expect(pagination.knownPageCount.value).toBe(1)
    expect(pagination.advance()).toBe(false)

    pagination.setNextCursor(2)
    expect(pagination.knownPageCount.value).toBe(2)
    pagination.advance()
    pagination.setNextCursor(undefined)

    expect(pagination.currentPage.value).toBe(2)
    expect(pagination.knownPageCount.value).toBe(2)
    expect(pagination.canAdvance.value).toBe(false)
  })

  it('treats zero and an empty string as legal cursor values', () => {
    const numberPagination = useAdminCursorPagination<number>()
    const stringPagination = useAdminCursorPagination<string>()

    expect(numberPagination.setNextCursor(0)).toBe(true)
    expect(numberPagination.advance()).toBe(true)
    expect(numberPagination.cursor.value).toBe(0)

    expect(stringPagination.setNextCursor('')).toBe(true)
    expect(stringPagination.advance()).toBe(true)
    expect(stringPagination.cursor.value).toBe('')
  })

  it('accepts zero but rejects invalid and cyclic numeric cursor values', () => {
    const pagination = useAdminCursorPagination<number>()

    expect(pagination.setNextCursor(0)).toBe(true)
    pagination.reset()

    expect(pagination.setNextCursor(-1)).toBe(false)
    expect(pagination.setNextCursor(1.5)).toBe(false)
    expect(pagination.setNextCursor(Number.MAX_SAFE_INTEGER + 1)).toBe(false)
    expect(pagination.setNextCursor(Number.NaN)).toBe(false)
    expect(pagination.setNextCursor(Number.POSITIVE_INFINITY)).toBe(false)
    expect(pagination.setNextCursor(Number.NEGATIVE_INFINITY)).toBe(false)
    expect(pagination.hasNext.value).toBe(false)

    expect(pagination.setNextCursor(2)).toBe(true)
    pagination.advance()

    expect(pagination.setNextCursor(2)).toBe(false)
    expect(pagination.hasNext.value).toBe(false)
    expect(pagination.advance()).toBe(false)
  })
})
