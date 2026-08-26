import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiResponseTaskData, Task } from './generated-query/model'
import {
  TaskCancelledError,
  waitTaskImageResultsWithSse,
} from './taskEvents'
import { subscribeUserCreditsInvalidation } from '../query/userCreditsInvalidation'

const mocks = vi.hoisted(() => ({
  getTask: vi.fn(),
}))

vi.mock('./task', () => ({
  taskApi: {
    get: mocks.getTask,
  },
}))

class MockEventSource {
  static instances: MockEventSource[] = []

  onerror: ((event: Event) => void) | null = null
  closed = false
  readonly url: string
  readonly init?: EventSourceInit
  private listeners: Record<string, EventListener[]> = {}

  constructor(url: string, init?: EventSourceInit) {
    this.url = url
    this.init = init
    MockEventSource.instances.push(this)
  }

  addEventListener(type: string, listener: EventListenerOrEventListenerObject) {
    const eventListener: EventListener =
      typeof listener === 'function' ? listener : listener.handleEvent.bind(listener)
    this.listeners[type] = [...(this.listeners[type] ?? []), eventListener]
  }

  removeEventListener(type: string, listener: EventListenerOrEventListenerObject) {
    const eventListener =
      typeof listener === 'function' ? listener : listener.handleEvent.bind(listener)
    this.listeners[type] = (this.listeners[type] ?? []).filter((entry) => entry !== eventListener)
  }

  close() {
    this.closed = true
  }

  emit(type: string, data: unknown) {
    const event = new MessageEvent(type, {
      data: typeof data === 'string' ? data : JSON.stringify(data),
    })
    ;(this.listeners[type] ?? []).forEach((listener) => listener(event))
  }

  fail() {
    this.onerror?.(new Event('error'))
  }
}

function makeTask(status: string, task: Partial<Task> = {}): Task {
  return {
    task_id: 'task-1',
    correlation_id: 'task-1',
    prompt: '',
    status,
    error_msg: '',
    cancel_reason: '',
    result: '',
    created_at: '2026-01-01T00:00:00.000Z',
    updated_at: '2026-01-01T00:00:00.000Z',
    completed_at: null,
    ...task,
  }
}

function makeImageOutput(url: string): NonNullable<Task['outputs']>[number] {
  return {
    id: 1,
    task_id: 'task-1',
    output_kind: 'asset',
    role: 'image',
    url,
    mime_type: 'image/png',
    width: 1024,
    height: 1024,
    size_bytes: 1024,
    result_index: 0,
    created_at: '2026-01-01T00:00:00.000Z',
    updated_at: '2026-01-01T00:00:00.000Z',
  }
}

function makeFinalizingFailureMetrics(
  overrides: Partial<NonNullable<Task['execution_metrics']>> = {},
): NonNullable<Task['execution_metrics']> {
  return {
    task_id: 'task-1',
    current_stage: 'failed',
    bottleneck_source: 'storage',
    failure_stage: 'finalizing',
    failure_category: 'storage_error',
    queue_wait_ms: 0,
    worker_overhead_ms: 0,
    provider_latency_ms: 100,
    finalize_latency_ms: 20,
    total_latency_ms: 120,
    created_at: '2026-01-01T00:00:00.000Z',
    updated_at: '2026-01-01T00:00:00.000Z',
    ...overrides,
  }
}

function makeTaskResponse(status: string, task: Partial<Task> = {}): ApiResponseTaskData {
  return {
    task: makeTask(status, task),
  }
}

describe('waitTaskImageResultsWithSse', () => {
  beforeEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.clearAllMocks()
    MockEventSource.instances = []
  })

  it('falls back to task polling when EventSource is unavailable', async () => {
    vi.stubGlobal('EventSource', undefined)
    mocks.getTask.mockResolvedValueOnce(
      makeTaskResponse('completed', {
        outputs: [makeImageOutput(' https://cdn.example.com/image.png ')],
      }),
    )

    await expect(waitTaskImageResultsWithSse('task-1')).resolves.toEqual([
      {
        url: 'https://cdn.example.com/image.png',
        width: 1024,
        height: 1024,
        mimeType: 'image/png',
        resultIndex: 0,
      },
    ])
    expect(mocks.getTask).toHaveBeenCalledWith('task-1')
  })

  it('resolves image results after a completed task invalidation', async () => {
    vi.stubGlobal('EventSource', MockEventSource)
    mocks.getTask.mockResolvedValueOnce(
      makeTaskResponse('completed', {
        outputs: [makeImageOutput(' https://cdn.example.com/sse-output.png ')],
      }),
    )

    const result = waitTaskImageResultsWithSse('task-1')
    expect(MockEventSource.instances).toHaveLength(1)
    MockEventSource.instances[0]?.emit('task.updated', { task_id: 'task-1', status: 'completed' })

    await expect(result).resolves.toEqual([
      {
        url: 'https://cdn.example.com/sse-output.png',
        width: 1024,
        height: 1024,
        mimeType: 'image/png',
        resultIndex: 0,
      },
    ])
    expect(mocks.getTask).toHaveBeenCalledOnce()
    expect(MockEventSource.instances[0]?.closed).toBe(true)
  })

  it('notifies wallet and ledger consumers after terminal settlement', async () => {
    vi.stubGlobal('EventSource', MockEventSource)
    mocks.getTask.mockResolvedValueOnce(
      makeTaskResponse('completed', {
        outputs: [makeImageOutput('https://cdn.example.com/settled.png')],
      }),
    )
    const listener = vi.fn()
    const unsubscribe = subscribeUserCreditsInvalidation(listener)

    const result = waitTaskImageResultsWithSse('task-1')
    MockEventSource.instances[0]?.emit('task.updated', { task_id: 'task-1', status: 'completed' })
    await result

    expect(listener).toHaveBeenCalledWith({
      source: 'task-settlement',
      taskId: 'task-1',
      status: 'completed',
    })
    unsubscribe()
  })

  it('rejects malformed SSE payloads without falling back to polling', async () => {
    vi.stubGlobal('EventSource', MockEventSource)

    const result = waitTaskImageResultsWithSse('task-1')
    expect(MockEventSource.instances).toHaveLength(1)
    MockEventSource.instances[0]?.emit('task.updated', 'not-json')

    await expect(result).rejects.toThrow('Malformed task event payload')
    expect(mocks.getTask).not.toHaveBeenCalled()
    expect(MockEventSource.instances[0]?.closed).toBe(true)
  })

  it('propagates the backend failure message from a failed SSE event without polling', async () => {
    vi.stubGlobal('EventSource', MockEventSource)
    mocks.getTask.mockResolvedValueOnce(makeTaskResponse('failed', { error_msg: ' 上游超时 ' }))

    const result = waitTaskImageResultsWithSse('task-1')
    expect(MockEventSource.instances).toHaveLength(1)
    MockEventSource.instances[0]?.emit('task.updated', { task_id: 'task-1', status: 'failed' })

    await expect(result).rejects.toThrow(/^上游超时$/)
    expect(mocks.getTask).toHaveBeenCalledOnce()
    expect(MockEventSource.instances[0]?.closed).toBe(true)
  })

  it('treats storage materialization failures from SSE as pending retry', async () => {
    vi.stubGlobal('EventSource', MockEventSource)
    mocks.getTask.mockResolvedValueOnce(
      makeTaskResponse('failed', {
        error_msg: ' 图片结果保存失败，请检查存储配置 ',
        execution_metrics: makeFinalizingFailureMetrics(),
      }),
    )

    const result = waitTaskImageResultsWithSse('task-1')
    MockEventSource.instances[0]?.emit('task.updated', { task_id: 'task-1', status: 'failed' })

    await expect(result).rejects.toMatchObject({ kind: 'pending' })
    expect(mocks.getTask).toHaveBeenCalledOnce()
    expect(MockEventSource.instances[0]?.closed).toBe(true)
  })

  it('falls back to polling with backoff after an EventSource error', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('EventSource', MockEventSource)
    mocks.getTask.mockResolvedValueOnce(makeTaskResponse('running')).mockResolvedValueOnce(
      makeTaskResponse('completed', {
        result: JSON.stringify({ image_url: 'https://cdn.example.com/done.png' }),
      }),
    )

    const result = waitTaskImageResultsWithSse('task-1', {
      timeoutMs: 10_000,
      pollInitialDelayMs: 50,
      pollMaxDelayMs: 50,
    })
    MockEventSource.instances[0]?.fail()

    await vi.advanceTimersByTimeAsync(50)

    await expect(result).resolves.toEqual([{ url: 'https://cdn.example.com/done.png' }])
    expect(mocks.getTask).toHaveBeenCalledTimes(2)
    expect(MockEventSource.instances[0]?.closed).toBe(true)
  })

  it('shares one user EventSource across concurrent task waits', async () => {
    vi.stubGlobal('EventSource', MockEventSource)
    mocks.getTask.mockResolvedValue(
      makeTaskResponse('completed', {
        outputs: [makeImageOutput('https://cdn.example.com/done.png')],
      }),
    )
    const first = waitTaskImageResultsWithSse('task-1')
    const second = waitTaskImageResultsWithSse('task-1')
    expect(MockEventSource.instances).toHaveLength(1)
    MockEventSource.instances[0]?.emit('task.updated', { task_id: 'task-1', status: 'completed' })
    await Promise.all([first, second])
  })

  it('rejects explicitly when a task is cancelled from the SSE stream', async () => {
    vi.stubGlobal('EventSource', MockEventSource)
    mocks.getTask.mockResolvedValueOnce(
      makeTaskResponse('cancelled', { cancel_reason: '用户取消' }),
    )

    const result = waitTaskImageResultsWithSse('task-1')
    MockEventSource.instances[0]?.emit('task.updated', { task_id: 'task-1', status: 'cancelled' })

    await expect(result).rejects.toThrow(TaskCancelledError)
    await expect(result).rejects.toThrow('用户取消')
    expect(mocks.getTask).toHaveBeenCalledOnce()
  })

  it('rejects explicitly when polling observes a cancelled task', async () => {
    vi.stubGlobal('EventSource', undefined)
    mocks.getTask.mockResolvedValueOnce(
      makeTaskResponse('cancelled', {
        cancel_reason: '账号已停用',
      }),
    )

    const result = waitTaskImageResultsWithSse('task-1')
    await expect(result).rejects.toThrow(TaskCancelledError)
    await expect(result).rejects.toThrow('账号已停用')
  })

  it('treats storage materialization failures from polling as pending retry', async () => {
    vi.stubGlobal('EventSource', undefined)
    mocks.getTask.mockResolvedValueOnce(
      makeTaskResponse('failed', {
        error_msg: '图片结果保存失败，请检查存储配置',
        execution_metrics: makeFinalizingFailureMetrics(),
      }),
    )

    const result = waitTaskImageResultsWithSse('task-1')

    await expect(result).rejects.toMatchObject({ kind: 'pending' })
  })
})
