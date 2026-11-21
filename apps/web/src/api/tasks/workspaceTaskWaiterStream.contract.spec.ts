import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { waitWorkspaceTaskTerminal } from './workspaceTaskWaiter'
import { resetWorkspaceEventStreamsForTests } from '../realtime/workspaceEventStream'

const mocks = vi.hoisted(() => ({ read: vi.fn() }))
vi.mock('./workspaceTaskTransport', () => ({
  getWorkspaceTask: mocks.read,
  isWorkspaceTaskGoneError: (error: unknown) => error instanceof Error && error.message === 'gone',
}))

const WORKSPACE_ID = 'ws_abc123'
const TASK_ID = 'task_abc123'
const running = { id: TASK_ID, workspace_id: WORKSPACE_ID, status: 'running' }

class HealthyEventSource {
  static instances: HealthyEventSource[] = []
  onerror: (() => void) | null = null
  isClosed = false
  readonly url: string
  private listeners = new Map<string, EventListener[]>()

  constructor(url: string) {
    this.url = url
    HealthyEventSource.instances.push(this)
  }

  addEventListener(type: string, listener: EventListener) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener])
  }

  close() {
    this.isClosed = true
  }

  open() {
    for (const listener of this.listeners.get('open') ?? []) listener(new Event('open'))
  }

  emit(type: string, taskId = TASK_ID, data: Record<string, unknown> = {}, eventId = 'evt_abc123') {
    const envelope = {
      id: eventId,
      type,
      version: 1,
      occurred_at: '2026-11-21T00:00:00Z',
      actor: { type: 'user', id: 'usr_abc123' },
      resource: { type: 'task', id: taskId },
      workspace_id: WORKSPACE_ID,
      data: { task_id: taskId, ...data },
    }
    const event = new MessageEvent(type, { data: JSON.stringify(envelope) })
    for (const listener of this.listeners.get(type) ?? []) listener(event)
  }
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(0)
  mocks.read.mockReset()
  HealthyEventSource.instances = []
  vi.stubGlobal('EventSource', HealthyEventSource)
})

afterEach(() => {
  resetWorkspaceEventStreamsForTests()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('workspace task stream reconciliation', () => {
  it('reconciles a task completed between initial GET snapshot and stream subscription', async () => {
    mocks.read
      .mockResolvedValueOnce(running)
      .mockResolvedValueOnce({ ...running, status: 'completed' })
    await expect(waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID)).resolves.toMatchObject({
      status: 'completed',
    })
    expect(mocks.read).toHaveBeenCalledTimes(2)
    expect(HealthyEventSource.instances[0]?.isClosed).toBe(true)
    expect(vi.getTimerCount()).toBe(0)
  })

  it.each(['completed', 'failed', 'cancelled'])(
    'reconciles %s committed after second GET before first SSE handshake',
    async (status) => {
      mocks.read.mockResolvedValue(running)
      const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, { deadlineAt: 60_000 })
      await vi.advanceTimersByTimeAsync(0)
      expect(mocks.read).toHaveBeenCalledTimes(2)
      mocks.read.mockResolvedValue({ ...running, status })
      HealthyEventSource.instances[0]?.open()
      await vi.advanceTimersByTimeAsync(14_999)
      expect(mocks.read).toHaveBeenCalledTimes(2)
      await vi.advanceTimersByTimeAsync(1)
      expect(mocks.read).toHaveBeenCalledTimes(3)
      await expect(pending).resolves.toMatchObject({ status })
      expect(HealthyEventSource.instances[0]?.isClosed).toBe(true)
      expect(vi.getTimerCount()).toBe(0)
      await vi.advanceTimersByTimeAsync(60_000)
      expect(mocks.read).toHaveBeenCalledTimes(3)
    },
  )

  it('bounds authoritative reads to one per 15 seconds on a healthy silent stream', async () => {
    mocks.read.mockResolvedValue(running)
    const controller = new AbortController()
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, {
      deadlineAt: 120_000,
      signal: controller.signal,
    })
    const rejected = expect(pending).rejects.toMatchObject({ name: 'AbortError' })
    await vi.advanceTimersByTimeAsync(44_999)
    expect(mocks.read).toHaveBeenCalledTimes(4)
    await vi.advanceTimersByTimeAsync(1)
    expect(mocks.read).toHaveBeenCalledTimes(5)
    expect(HealthyEventSource.instances).toHaveLength(1)
    controller.abort()
    await rejected
    expect(HealthyEventSource.instances[0]?.isClosed).toBe(true)
    expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(120_000)
    expect(mocks.read).toHaveBeenCalledTimes(5)
  })

  it('uses successful event reads to postpone redundant timed reconciliation', async () => {
    mocks.read.mockResolvedValue(running)
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, { deadlineAt: 60_000 })
    await vi.advanceTimersByTimeAsync(14_000)
    HealthyEventSource.instances[0]?.emit('task.running')
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.read).toHaveBeenCalledTimes(3)
    await vi.advanceTimersByTimeAsync(14_999)
    expect(mocks.read).toHaveBeenCalledTimes(3)
    mocks.read.mockResolvedValue({ ...running, status: 'completed' })
    await vi.advanceTimersByTimeAsync(1)
    await expect(pending).resolves.toMatchObject({ status: 'completed' })
    expect(mocks.read).toHaveBeenCalledTimes(4)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('does not let unrelated task events postpone reconciliation', async () => {
    mocks.read.mockResolvedValue(running)
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, { deadlineAt: 60_000 })
    await vi.advanceTimersByTimeAsync(14_000)
    HealthyEventSource.instances[0]?.emit('task.running', 'task_other123')
    expect(mocks.read).toHaveBeenCalledTimes(2)
    mocks.read.mockResolvedValue({ ...running, status: 'failed' })
    await vi.advanceTimersByTimeAsync(1_000)
    await expect(pending).resolves.toMatchObject({ status: 'failed' })
    expect(mocks.read).toHaveBeenCalledTimes(3)
  })

  it('does not postpone reconciliation when event-triggered reads fail', async () => {
    mocks.read.mockResolvedValue(running)
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, { deadlineAt: 60_000 })
    await vi.advanceTimersByTimeAsync(14_000)
    mocks.read.mockRejectedValueOnce(new Error('temporary read failure'))
    HealthyEventSource.instances[0]?.emit('task.running')
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.read).toHaveBeenCalledTimes(3)
    mocks.read.mockResolvedValue({ ...running, status: 'cancelled' })
    await vi.advanceTimersByTimeAsync(1_000)
    await expect(pending).resolves.toMatchObject({ status: 'cancelled' })
    expect(mocks.read).toHaveBeenCalledTimes(4)
  })

  it('does not queue timer reads behind a slow authoritative GET', async () => {
    let release: ((item: typeof running) => void) | undefined
    mocks.read.mockResolvedValue(running)
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, { deadlineAt: 90_000 })
    await vi.advanceTimersByTimeAsync(14_000)
    mocks.read.mockImplementationOnce(
      () =>
        new Promise<typeof running>((resolve) => {
          release = resolve
        }),
    )
    HealthyEventSource.instances[0]?.emit('task.running')
    await vi.advanceTimersByTimeAsync(31_000)
    expect(mocks.read).toHaveBeenCalledTimes(3)
    release?.(running)
    await vi.advanceTimersByTimeAsync(0)
    mocks.read.mockResolvedValue({ ...running, status: 'completed' })
    await vi.advanceTimersByTimeAsync(14_999)
    expect(mocks.read).toHaveBeenCalledTimes(3)
    await vi.advanceTimersByTimeAsync(1)
    await expect(pending).resolves.toMatchObject({ status: 'completed' })
    expect(mocks.read).toHaveBeenCalledTimes(4)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('waits for a slow timed read before starting one disconnect polling loop', async () => {
    let release: ((item: typeof running) => void) | undefined
    mocks.read.mockResolvedValue(running)
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, {
      deadlineAt: 90_000,
      pollInitialDelayMs: 1_000,
    })
    await vi.advanceTimersByTimeAsync(0)
    mocks.read.mockImplementationOnce(
      () =>
        new Promise<typeof running>((resolve) => {
          release = resolve
        }),
    )
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mocks.read).toHaveBeenCalledTimes(3)
    HealthyEventSource.instances[0]?.onerror?.()
    HealthyEventSource.instances[0]?.onerror?.()
    await vi.advanceTimersByTimeAsync(30_000)
    expect(mocks.read).toHaveBeenCalledTimes(3)
    release?.(running)
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.read).toHaveBeenCalledTimes(4)
    mocks.read.mockResolvedValue({ ...running, status: 'completed' })
    await vi.advanceTimersByTimeAsync(1_000)
    await expect(pending).resolves.toMatchObject({ status: 'completed' })
    expect(mocks.read).toHaveBeenCalledTimes(5)
    expect(HealthyEventSource.instances).toHaveLength(1)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('coalesces matching events during a timed GET into one follow-up read', async () => {
    let release: ((item: typeof running) => void) | undefined
    mocks.read.mockResolvedValue(running)
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, { deadlineAt: 90_000 })
    await vi.advanceTimersByTimeAsync(0)
    mocks.read.mockImplementationOnce(
      () =>
        new Promise<typeof running>((resolve) => {
          release = resolve
        }),
    )
    await vi.advanceTimersByTimeAsync(15_000)
    for (let index = 0; index < 20; index++) {
      HealthyEventSource.instances[0]?.emit('task.running', TASK_ID, {}, `evt_hint${index}`)
    }
    expect(mocks.read).toHaveBeenCalledTimes(3)
    release?.(running)
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.read).toHaveBeenCalledTimes(4)
    mocks.read.mockResolvedValue({ ...running, status: 'completed' })
    await vi.advanceTimersByTimeAsync(15_000)
    await expect(pending).resolves.toMatchObject({ status: 'completed' })
    expect(mocks.read).toHaveBeenCalledTimes(5)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('reuses a terminal in-flight snapshot after disconnect without another GET', async () => {
    let release: ((item: typeof running) => void) | undefined
    mocks.read.mockResolvedValue(running)
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, { deadlineAt: 90_000 })
    await vi.advanceTimersByTimeAsync(0)
    mocks.read.mockImplementationOnce(
      () =>
        new Promise<typeof running>((resolve) => {
          release = resolve
        }),
    )
    await vi.advanceTimersByTimeAsync(15_000)
    HealthyEventSource.instances[0]?.onerror?.()
    release?.({ ...running, status: 'completed' })
    await expect(pending).resolves.toMatchObject({ status: 'completed' })
    expect(mocks.read).toHaveBeenCalledTimes(3)
    expect(HealthyEventSource.instances[0]?.isClosed).toBe(true)
    expect(vi.getTimerCount()).toBe(0)
  })

  it.each([false, true])(
    'aborts a timed GET after disconnect=%s without scheduling late work',
    async (shouldDisconnect) => {
      let release: ((item: typeof running) => void) | undefined
      mocks.read.mockResolvedValue(running)
      const controller = new AbortController()
      const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, {
        deadlineAt: 90_000,
        signal: controller.signal,
      })
      const rejected = expect(pending).rejects.toMatchObject({ name: 'AbortError' })
      await vi.advanceTimersByTimeAsync(0)
      mocks.read.mockImplementationOnce(
        () =>
          new Promise<typeof running>((resolve) => {
            release = resolve
          }),
      )
      await vi.advanceTimersByTimeAsync(15_000)
      const signal = mocks.read.mock.calls[2]?.[2].signal as AbortSignal
      if (shouldDisconnect) HealthyEventSource.instances[0]?.onerror?.()
      controller.abort()
      await rejected
      expect(signal.aborted).toBe(true)
      expect(HealthyEventSource.instances[0]?.isClosed).toBe(true)
      release?.(running)
      await vi.advanceTimersByTimeAsync(90_000)
      expect(mocks.read).toHaveBeenCalledTimes(3)
      expect(vi.getTimerCount()).toBe(0)
    },
  )

  it('keeps one absolute deadline while a healthy stream has no terminal notification', async () => {
    mocks.read.mockResolvedValue(running)
    const pending = waitWorkspaceTaskTerminal(WORKSPACE_ID, TASK_ID, { deadlineAt: 40_000 })
    const rejected = expect(pending).rejects.toMatchObject({ name: 'TimeoutError' })
    await vi.advanceTimersByTimeAsync(40_000)
    await rejected
    expect(mocks.read).toHaveBeenCalledTimes(4)
    expect(HealthyEventSource.instances[0]?.isClosed).toBe(true)
    expect(vi.getTimerCount()).toBe(0)
  })
})
