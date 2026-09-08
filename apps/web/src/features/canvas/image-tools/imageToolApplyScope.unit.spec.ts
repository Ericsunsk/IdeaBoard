import { describe, expect, it, vi } from 'vitest'
import { createImageToolApplyScopeHost } from './imageToolApplyScope'

function createHost(overrides: { boardId?: string | null; alive?: boolean } = {}) {
  const boardId = vi.fn(() => overrides.boardId ?? 'board-1')
  const isNodeAlive = vi.fn(() => overrides.alive ?? true)
  return { host: createImageToolApplyScopeHost({ currentBoardId: boardId, isNodeAlive }), boardId }
}

describe('imageToolApplyScope', () => {
  it('rejects a second begin while applying', () => {
    const { host } = createHost()
    const first = host.begin('image-1', 'board-1')
    expect(first).not.toBeNull()
    expect(host.begin('image-1', 'board-1')).toBeNull()
    expect(host.isApplying.value).toBe(true)
  })

  it('treats a superseded scope as stale after cancel and re-begin', () => {
    const { host } = createHost()
    const first = host.begin('image-1', 'board-1')
    host.cancel('image-1')
    const second = host.begin('image-1', 'board-1')
    expect(first?.isCurrent()).toBe(false)
    expect(second?.isCurrent()).toBe(true)
  })

  it('goes stale when the board changes, the node dies, or the scope id moves', () => {
    const { host, boardId } = createHost()
    const scope = host.begin('image-1', 'board-1')
    boardId.mockReturnValue('board-2')
    expect(scope?.isCurrent()).toBe(false)
    boardId.mockReturnValue('board-1')

    const { host: deadHost } = createHost({ alive: false })
    expect(deadHost.begin('image-1', 'board-1')?.isCurrent()).toBe(false)

    const { host: movedHost } = createHost()
    const moved = movedHost.begin('image-1', 'board-1')
    movedHost.cancel('image-2')
    expect(moved?.isCurrent()).toBe(true)
    movedHost.cancel('image-1')
    expect(moved?.isCurrent()).toBe(false)
  })

  it('runs the registered cleanup exactly once on cancel and release', () => {
    const { host } = createHost()
    const scope = host.begin('image-1', 'board-1')
    const cleanup = vi.fn()
    host.onCleanup(cleanup)
    host.cancel('image-1')
    host.cancel('image-1')
    expect(cleanup).toHaveBeenCalledTimes(1)
    expect(host.isApplying.value).toBe(false)
    expect(host.scopeId.value).toBeNull()

    const again = host.begin('image-1', 'board-1')
    const secondCleanup = vi.fn()
    host.onCleanup(secondCleanup)
    again?.release()
    again?.release()
    expect(secondCleanup).toHaveBeenCalledTimes(1)
    expect(scope).not.toBeNull()
  })

  it('replacing the cleanup runs the previous one first', () => {
    const { host } = createHost()
    host.begin('image-1', 'board-1')
    const first = vi.fn()
    const second = vi.fn()
    host.onCleanup(first)
    host.onCleanup(second)
    expect(first).toHaveBeenCalledTimes(1)
    expect(second).not.toHaveBeenCalled()
  })

  it('release is a no-op for a superseded generation', () => {
    const { host } = createHost()
    const first = host.begin('image-1', 'board-1')
    const cleanup = vi.fn()
    host.onCleanup(cleanup)
    host.cancel('image-1')
    const second = host.begin('image-1', 'board-1')
    first?.release()
    expect(cleanup).toHaveBeenCalledTimes(1)
    expect(second?.isCurrent()).toBe(true)
    expect(host.isApplying.value).toBe(true)
  })

  it('tracks the pending node until explicitly cleared', () => {
    const { host } = createHost()
    host.begin('image-1', 'board-1')
    expect(host.pendingNodeId.value).toBeNull()
    host.notePendingNode('image-3')
    expect(host.pendingNodeId.value).toBe('image-3')
    host.clearPendingNode('image-9')
    expect(host.pendingNodeId.value).toBe('image-3')
    host.clearPendingNode('image-3')
    expect(host.pendingNodeId.value).toBeNull()
  })
})
