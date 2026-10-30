import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { waitWorkspaceTaskTerminal } from './workspaceTaskWaiter'
import type { WorkspaceTaskItem } from './workspaceTaskContract'

const mocks = vi.hoisted(() => ({
  read: vi.fn(),
  subscribe: vi.fn<(...args: unknown[]) => () => void>(() => () => undefined),
}))
vi.mock('./workspaceTaskTransport', () => ({
  getWorkspaceTask: mocks.read,
  isWorkspaceTaskGoneError: (error: unknown) => error instanceof Error && error.message === 'gone',
}))
vi.mock('./workspaceEventStream', () => ({ subscribeWorkspaceEvent: mocks.subscribe }))

const WORKSPACE_ID = 'ws_aaa111'
const TASK_ID = 'task_bbb222'
const running = { id: TASK_ID, status: 'running' } satisfies Partial<WorkspaceTaskItem>

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(0)
  vi.clearAllMocks()
  mocks.read.mockReset()
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('workspace task whole-operation deadline', () => {
  it('refreshes a live task from matching stream hints and releases the subscription', async () => {
    vi.stubGlobal('EventSource', class {})
    const unsubscribe = vi.fn()
    mocks.subscribe.mockReturnValueOnce(unsubscribe)
    mocks.read
      .mockResolvedValueOnce(running)
      .mockResolvedValueOnce(running)
      .mockResolvedValueOnce({ id: TASK_ID, status: 'completed' })
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, { deadlineAt: 1_000 })
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.subscribe).toHaveBeenCalledOnce()
    expect(mocks.read).toHaveBeenCalledTimes(2)
    const onEvent = mocks.subscribe.mock.calls[0]?.[1]
    if (typeof onEvent !== 'function') throw new Error('Expected a stream listener')
    onEvent({ data: { task_id: 'task_other' } })
    expect(mocks.read).toHaveBeenCalledTimes(2)
    onEvent({ data: { task_id: TASK_ID } })
    await expect(pending).resolves.toMatchObject({ status: 'completed' })
    expect(unsubscribe).toHaveBeenCalledOnce()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('reads authoritative Task facts when completion arrives without a prior snapshot', async () => {
    vi.stubGlobal('EventSource', class {})
    const completed = {
      id: TASK_ID,
      service: 'render',
      status: 'completed',
      billing_snapshot: { sku: 'ai-image:render', quantity: 4, settlement_state: 'captured' },
    }
    mocks.read
      .mockRejectedValueOnce(new Error('initial read unavailable'))
      .mockRejectedValueOnce(new Error('reconciliation unavailable'))
      .mockResolvedValueOnce(completed)
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, { deadlineAt: 1_000 })
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.read).toHaveBeenCalledTimes(2)
    const onEvent = mocks.subscribe.mock.calls[0]?.[1]
    if (typeof onEvent !== 'function') throw new Error('Expected a stream listener')
    onEvent({
      type: 'task.completed',
      occurred_at: '2026-10-30T00:00:00Z',
      data: { task_id: TASK_ID, outputs: [{ asset_id: 'asset_result' }] },
    })

    await expect(pending).resolves.toBe(completed)
    expect(mocks.read).toHaveBeenCalledTimes(3)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('retains completion received while the first authoritative snapshot is in flight', async () => {
    vi.stubGlobal('EventSource', class {})
    const unsubscribe = vi.fn()
    mocks.subscribe.mockReturnValueOnce(unsubscribe)
    const completed = {
      id: TASK_ID,
      service: 'render',
      status: 'completed',
      billing_snapshot: { sku: 'ai-image:render', quantity: 4, settlement_state: 'captured' },
    }
    let releaseReconciliation: ((item: typeof running) => void) | undefined
    const reconciliation = new Promise<typeof running>((resolve) => {
      releaseReconciliation = resolve
    })
    mocks.read
      .mockRejectedValueOnce(new Error('initial read unavailable'))
      .mockReturnValueOnce(reconciliation)
      .mockResolvedValueOnce(completed)
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, { deadlineAt: 1_000 })
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.read).toHaveBeenCalledTimes(2)
    const onEvent = mocks.subscribe.mock.calls[0]?.[1]
    if (typeof onEvent !== 'function') throw new Error('Expected a stream listener')
    onEvent({
      type: 'task.completed',
      occurred_at: '2026-10-30T00:00:00Z',
      data: { task_id: TASK_ID, outputs: [{ asset_id: 'asset_result' }] },
    })
    expect(mocks.read).toHaveBeenCalledTimes(2)
    releaseReconciliation?.(running)

    await expect(pending).resolves.toBe(completed)
    expect(mocks.read).toHaveBeenCalledTimes(3)
    expect(unsubscribe).toHaveBeenCalledOnce()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('bounds a hanging fast GET and aborts its transport', async () => {
    mocks.read.mockImplementation(() => new Promise(() => undefined))
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, { deadlineAt: 100 })
    const rejected = expect(pending).rejects.toMatchObject({ name: 'TimeoutError' })
    await vi.advanceTimersByTimeAsync(100)
    await rejected
    expect(mocks.read.mock.calls[0]?.[2].signal.aborted).toBe(true)
    expect(mocks.subscribe).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('does not restart the deadline when the initial GET falls through to polling', async () => {
    vi.stubGlobal('EventSource', undefined)
    mocks.read
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            setTimeout(() => resolve(running), 80)
          }),
      )
      .mockImplementation(() => new Promise(() => undefined))
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, { deadlineAt: 100 })
    const rejected = expect(pending).rejects.toMatchObject({ name: 'TimeoutError' })
    await vi.advanceTimersByTimeAsync(80)
    expect(mocks.read).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(20)
    await rejected
    expect(Date.now()).toBe(100)
    expect(mocks.read.mock.calls[1]?.[2].signal.aborted).toBe(true)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('aborts polling sleep immediately and performs no further read', async () => {
    vi.stubGlobal('EventSource', undefined)
    mocks.read.mockResolvedValue(running)
    const controller = new AbortController()
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, {
      deadlineAt: 10_000,
      pollInitialDelayMs: 1_000,
      signal: controller.signal,
    })
    const rejected = expect(pending).rejects.toMatchObject({ name: 'AbortError' })
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.read).toHaveBeenCalledTimes(2)
    controller.abort()
    await rejected
    await vi.advanceTimersByTimeAsync(10_000)
    expect(mocks.read).toHaveBeenCalledTimes(2)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('rejects an already expired deadline without starting a read', async () => {
    await expect(
      waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, { deadlineAt: 0 }),
    ).rejects.toMatchObject({ name: 'TimeoutError' })
    expect(mocks.read).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
  })
})
