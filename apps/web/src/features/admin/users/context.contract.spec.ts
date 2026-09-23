import { nextTick, ref, toValue, type MaybeRefOrGetter } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminUsersFeedback } from './useUsersFeedback'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import { useAdminUsersContext } from './context'
import type { AdminUsersPage, AdminUsersQueryFilters } from './resource'

const mockState = vi.hoisted(() => ({
  filtersRef: undefined as MaybeRefOrGetter<AdminUsersQueryFilters> | undefined,
  collection: undefined as unknown,
}))

function currentFilters(): AdminUsersQueryFilters | undefined {
  if (mockState.filtersRef === undefined) return undefined
  return toValue(mockState.filtersRef)
}

vi.mock('./resource', () => ({
  useAdminUsersResource: vi.fn((filters: MaybeRefOrGetter<AdminUsersQueryFilters>) => {
    mockState.filtersRef = filters
    return mockState.collection
  }),
}))

vi.mock('./commands', () => ({
  useAdminUserCommands: vi.fn(() => ({})),
}))

type CollectionStub = {
  data: ReturnType<typeof ref<AdminUsersPage | undefined>>
  loading: ReturnType<typeof ref<boolean>>
  initialError: ReturnType<typeof ref<boolean>>
  backgroundError: ReturnType<typeof ref<boolean>>
  refresh: ReturnType<typeof vi.fn>
  query: { error: ReturnType<typeof ref<Error | null>> }
  pagination: {
    currentPage: ReturnType<typeof ref<number>>
    knownPageCount: ReturnType<typeof ref<number>>
    canRetreat: ReturnType<typeof ref<boolean>>
    canAdvance: ReturnType<typeof ref<boolean>>
    reset: ReturnType<typeof vi.fn>
  }
  previousPage: ReturnType<typeof vi.fn>
  nextPage: ReturnType<typeof vi.fn>
}

function makeCollection(): CollectionStub {
  return {
    data: ref<AdminUsersPage | undefined>({
      items: [],
      nextCursor: 'cursor-2',
      hasMore: true,
    }),
    loading: ref(false),
    initialError: ref(false),
    backgroundError: ref(false),
    refresh: vi.fn(),
    query: { error: ref<Error | null>(null) },
    pagination: {
      currentPage: ref(1),
      knownPageCount: ref(2),
      canRetreat: ref(false),
      canAdvance: ref(true),
      reset: vi.fn(),
    },
    previousPage: vi.fn(),
    nextPage: vi.fn(),
  }
}

beforeEach(() => {
  mockState.filtersRef = undefined
  mockState.collection = makeCollection()
})

describe('admin users cursor pagination contract', () => {
  it('navigates through known cursor pages without totals', () => {
    const context = createContext()
    const pagination = context.list.pagination

    expect(pagination.currentPage.value).toBe(1)
    expect(pagination.totalPages.value).toBe(2)
    pagination.next()
    expect((mockState.collection as CollectionStub).nextPage).toHaveBeenCalled()
    pagination.previous()
    expect((mockState.collection as CollectionStub).previousPage).toHaveBeenCalled()
  })

  it('resets cursor state when filters change and applies keyword on load', async () => {
    const context = createContext()
    const collection = mockState.collection as CollectionStub

    context.list.userRoleFilter.value = 'admin'
    await nextTick()
    expect(collection.pagination.reset).toHaveBeenCalled()

    context.list.searchKeyword.value = '  alice  '
    context.list.loadUsers()
    await nextTick()
    expect(collection.pagination.reset).toHaveBeenCalled()
    expect(currentFilters()?.keyword).toBe('alice')
  })

  it('refetches without touching filters when the keyword is unchanged', () => {
    const context = createContext()
    const collection = mockState.collection as CollectionStub

    context.list.loadUsers()
    expect(collection.refresh).toHaveBeenCalled()
  })

  it('reports collection errors through feedback', async () => {
    const feedback = { loadFailed: vi.fn() }
    createContext(feedback)
    const collection = mockState.collection as CollectionStub

    collection.initialError.value = true
    await nextTick()
    expect(feedback.loadFailed).toHaveBeenCalled()
  })
})

function createContext(
  feedback: { loadFailed: ReturnType<typeof vi.fn> } = { loadFailed: vi.fn() },
) {
  return useAdminUsersContext({
    feedback: feedback as unknown as AdminUsersFeedback,
    confirm: { confirm: vi.fn().mockResolvedValue(true) } as AdminConfirmPort,
  })
}
