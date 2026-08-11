import { describe, expect, it, vi } from 'vitest'
import { useAdminCollectionState } from './useAdminCollectionState'

type Page = { items: string[]; total: number }

describe('useAdminCollectionState', () => {
  it('keeps the initial loading surface stable until the first result succeeds', async () => {
    const load = vi.fn<() => Promise<Page>>().mockResolvedValue({ items: ['paid'], total: 1 })
    const collection = useAdminCollectionState({ load, isEmpty: (page) => page.items.length === 0 })

    expect(collection.loading.value).toBe(true)
    expect(collection.initialError.value).toBe(false)
    expect(collection.data.value).toBeUndefined()

    await collection.reload()

    expect(collection.loading.value).toBe(false)
    expect(collection.initialError.value).toBe(false)
    expect(collection.data.value).toEqual({ items: ['paid'], total: 1 })
    expect(collection.hasLoadedResult.value).toBe(true)
    expect(collection.isEmpty.value).toBe(false)
  })

  it('publishes an actionable initial failure and lets retry replace it with a result', async () => {
    const load = vi
      .fn<() => Promise<Page>>()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce({ items: ['retried'], total: 1 })
    const collection = useAdminCollectionState({ load, isEmpty: (page) => page.items.length === 0 })

    await expect(collection.reload()).resolves.toBeUndefined()
    expect(collection.initialError.value).toBe(true)
    expect(collection.backgroundError.value).toBe(false)

    await collection.reload()
    expect(collection.initialError.value).toBe(false)
    expect(collection.data.value?.items).toEqual(['retried'])
  })

  it('publishes a successful empty result without an error surface', async () => {
    const collection = useAdminCollectionState({
      load: vi.fn<() => Promise<Page>>().mockResolvedValue({ items: [], total: 0 }),
      isEmpty: (page) => page.items.length === 0,
    })

    await collection.reload()

    expect(collection.hasLoadedResult.value).toBe(true)
    expect(collection.isEmpty.value).toBe(true)
    expect(collection.initialError.value).toBe(false)
  })

  it('lets a resource-specific row action update retained data without owning lifecycle state', async () => {
    const collection = useAdminCollectionState({
      load: vi.fn<() => Promise<Page>>().mockResolvedValue({ items: ['enabled'], total: 1 }),
      isEmpty: (page) => page.items.length === 0,
    })
    await collection.reload()

    collection.updateData((page) => ({ ...page, items: ['disabled'] }))

    expect(collection.data.value?.items).toEqual(['disabled'])
    expect(collection.hasLoadedResult.value).toBe(true)
  })

  it('retains successful data and reports a background refresh failure', async () => {
    const load = vi
      .fn<() => Promise<Page>>()
      .mockResolvedValueOnce({ items: ['retained'], total: 1 })
      .mockRejectedValueOnce(new Error('offline'))
    const collection = useAdminCollectionState({ load, isEmpty: (page) => page.items.length === 0 })

    await collection.reload()
    await collection.reload({ preserveData: true })

    expect(collection.data.value?.items).toEqual(['retained'])
    expect(collection.initialError.value).toBe(false)
    expect(collection.backgroundError.value).toBe(true)
  })

  it('cancels in-flight work without publishing its completion', async () => {
    let resolveRequest: (page: Page) => void = () => undefined
    const load = vi.fn(
      (signal: AbortSignal) =>
        new Promise<Page>((resolve) => {
          resolveRequest = resolve
          expect(signal.aborted).toBe(false)
        }),
    )
    const collection = useAdminCollectionState({ load, isEmpty: (page) => page.items.length === 0 })

    const request = collection.reload()
    collection.cancel()
    resolveRequest({ items: ['late'], total: 1 })
    await request

    expect(load.mock.calls[0]?.[0].aborted).toBe(true)
    expect(collection.loading.value).toBe(false)
    expect(collection.data.value).toBeUndefined()
    expect(collection.initialError.value).toBe(false)
  })

  it('rejects stale completion after a newer request succeeds', async () => {
    let resolveOld: (page: Page) => void = () => undefined
    const load = vi
      .fn<(signal: AbortSignal) => Promise<Page>>()
      .mockImplementationOnce(
        () =>
          new Promise<Page>((resolve) => {
            resolveOld = resolve
          }),
      )
      .mockResolvedValueOnce({ items: ['newest'], total: 1 })
    const collection = useAdminCollectionState({ load, isEmpty: (page) => page.items.length === 0 })

    const oldRequest = collection.reload()
    await collection.reload()
    resolveOld({ items: ['stale'], total: 1 })
    await oldRequest

    expect(load.mock.calls[0]?.[0].aborted).toBe(true)
    expect(collection.data.value?.items).toEqual(['newest'])
  })
})
