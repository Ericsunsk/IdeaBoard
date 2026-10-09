import { describe, expect, it, vi } from 'vitest'
import { useOptimisticToggle } from './useOptimisticToggle'

describe('useOptimisticToggle', () => {
  it('flips intent immediately and stays pending until mutate resolves', async () => {
    const toggle = useOptimisticToggle<string>()
    let resolveMutate!: () => void
    const mutate = vi.fn(() => new Promise<void>((resolve) => (resolveMutate = resolve)))

    const promise = toggle.toggle('unsplash', true, mutate)
    expect(toggle.effective('unsplash', false)).toBe(true)
    expect(toggle.isPending('unsplash')).toBe(true)

    resolveMutate()
    await expect(promise).resolves.toBe(true)
    expect(toggle.isPending('unsplash')).toBe(false)
    // 成功不清覆盖：等父级回填 reconcile 才清，避免二次跳变。
    expect(toggle.effective('unsplash', false)).toBe(true)
  })

  it('reconciles the overlay once the server catches up', async () => {
    const toggle = useOptimisticToggle<string>()
    await toggle.toggle('unsplash', true, async () => {})
    toggle.reconcile('unsplash', true)
    expect(toggle.effective('unsplash', true)).toBe(true)
    expect(toggle.optimistic.value.has('unsplash')).toBe(false)
  })

  it('keeps the overlay when the server has not caught up yet', async () => {
    const toggle = useOptimisticToggle<string>()
    await toggle.toggle('unsplash', true, async () => {})
    toggle.reconcile('unsplash', false)
    expect(toggle.effective('unsplash', false)).toBe(true)
  })

  it('rolls back the intent on failure and reports via caller feedback', async () => {
    const toggle = useOptimisticToggle<string>()
    const failure = new Error('toggle-rollback')
    const onFailure = vi.fn()
    const onSuccess = vi.fn()

    const ok = await toggle.toggle('unsplash', true, async () => Promise.reject(failure), {
      onSuccess,
      onFailure,
    })

    expect(ok).toBe(false)
    expect(toggle.effective('unsplash', false)).toBe(false)
    expect(toggle.isPending('unsplash')).toBe(false)
    expect(onSuccess).not.toHaveBeenCalled()
    expect(onFailure).toHaveBeenCalledWith('unsplash', true, failure)
  })

  it('reports success via caller feedback without touching toast', async () => {
    const toggle = useOptimisticToggle<string>()
    const onSuccess = vi.fn()
    const ok = await toggle.toggle('unsplash', false, async () => {}, { onSuccess })
    expect(ok).toBe(true)
    expect(onSuccess).toHaveBeenCalledWith('unsplash', false)
  })

  it('guards concurrent toggles on the same key', async () => {
    const toggle = useOptimisticToggle<string>()
    let resolveFirst!: () => void
    const firstMutate = vi.fn(() => new Promise<void>((resolve) => (resolveFirst = resolve)))
    const secondMutate = vi.fn(async () => {})

    const first = toggle.toggle('unsplash', true, firstMutate)
    const second = await toggle.toggle('unsplash', false, secondMutate)
    expect(second).toBe(false)
    expect(secondMutate).not.toHaveBeenCalled()

    resolveFirst()
    await expect(first).resolves.toBe(true)
  })

  it('reconciles a batch of server rows', async () => {
    const toggle = useOptimisticToggle<string>()
    await toggle.toggle('unsplash', true, async () => {})
    toggle.reconcileAll([
      { key: 'unsplash', enabled: true },
      { key: 'pexels', enabled: false },
    ])
    expect(toggle.optimistic.value.has('unsplash')).toBe(false)
  })
})
