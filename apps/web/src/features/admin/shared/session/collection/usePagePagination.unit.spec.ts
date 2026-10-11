import { nextTick, ref, watch } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { usePagePagination } from './usePagePagination'

describe('usePagePagination', () => {
  it('calculates page bounds and navigates within them', () => {
    const currentPage = ref(1)
    const pagination = usePagePagination({
      currentPage,
      totalRecords: ref(21),
      pageSize: ref(10),
    })

    expect(pagination.totalPages.value).toBe(3)
    expect(pagination.canPrevious.value).toBe(false)
    expect(pagination.canNext.value).toBe(true)
    expect(pagination.previous()).toBe(false)

    expect(pagination.next()).toBe(true)
    expect(pagination.currentPage.value).toBe(2)
    expect(pagination.previous()).toBe(true)
    expect(pagination.currentPage.value).toBe(1)
  })

  it('clamps the current page once when the record count shrinks', async () => {
    const currentPage = ref(4)
    const totalRecords = ref(40)
    const pageChanges = vi.fn()
    const pagination = usePagePagination({
      currentPage,
      totalRecords,
      pageSize: ref(10),
    })
    watch(currentPage, pageChanges)

    totalRecords.value = 11
    await nextTick()

    expect(pagination.totalPages.value).toBe(2)
    expect(pagination.currentPage.value).toBe(2)
    expect(pageChanges).toHaveBeenCalledOnce()
    expect(pageChanges.mock.calls[0]?.slice(0, 2)).toEqual([2, 4])
  })

  it('blocks both navigation directions while pending but still permits an explicit reset', () => {
    const currentPage = ref(2)
    const pending = ref(true)
    const pagination = usePagePagination({
      currentPage,
      totalRecords: ref(30),
      pageSize: ref(10),
      pending,
    })

    expect(pagination.canPrevious.value).toBe(false)
    expect(pagination.canNext.value).toBe(false)
    expect(pagination.previous()).toBe(false)
    expect(pagination.next()).toBe(false)
    expect(pagination.currentPage.value).toBe(2)

    expect(pagination.reset()).toBe(true)
    expect(pagination.currentPage.value).toBe(1)
    expect(pagination.reset()).toBe(false)
  })

  it('keeps the last known boundary when the current query has no total', () => {
    const currentPage = ref(3)
    const totalRecords = ref<number | undefined>(30)
    const pagination = usePagePagination({
      currentPage,
      totalRecords,
      pageSize: ref(10),
    })

    totalRecords.value = undefined

    expect(pagination.currentPage.value).toBe(3)
    expect(pagination.totalPages.value).toBe(3)

    totalRecords.value = 5

    expect(pagination.currentPage.value).toBe(1)
    expect(pagination.totalPages.value).toBe(1)
  })

  it('normalizes empty totals and invalid page sizes to one safe page', () => {
    const pagination = usePagePagination({
      currentPage: ref(1),
      totalRecords: ref(Number.NaN),
      pageSize: ref(0),
    })

    expect(pagination.totalPages.value).toBe(1)
    expect(pagination.canNext.value).toBe(false)
    expect(pagination.next()).toBe(false)

    const fractionalPageSize = usePagePagination({
      currentPage: ref(0),
      totalRecords: ref(2),
      pageSize: ref(0.5),
    })
    expect(fractionalPageSize.currentPage.value).toBe(1)
    expect(fractionalPageSize.totalPages.value).toBe(2)
  })
})
