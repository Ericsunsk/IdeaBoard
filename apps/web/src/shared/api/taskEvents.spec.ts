import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiResponseTask, Task } from './generated-query/model'
import { queryClient } from '../query/queryClient'
import { waitTaskResultWithSse } from './taskEvents'

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
    id: 1,
    task_id: 'task-1',
    user_id: 1,
    prompt: '',
    status,
    created_at: '2026-01-01T00:00:00.000Z',
    updated_at: '2026-01-01T00:00:00.000Z',
    ...task,
  }
}

function makeImageOutput(url: string): NonNullable<Task['outputs']>[number] {
  return {
    id: 1,
    task_id: 'task-1',
    role: 'image',
    url,
    storage_key: 'tasks/task-1/image.png',
    mime_type: 'image/png',
    width: 1024,
    height: 1024,
    size_bytes: 1024,
    sort_index: 0,
    created_at: '2026-01-01T00:00:00.000Z',
    updated_at: '2026-01-01T00:00:00.000Z',
  }
}

function makeTaskResponse(status: string, task: Partial<Task> = {}): ApiResponseTask {
  return {
    code: 0,
    msg: 'ok',
    data: {
      task: makeTask(status, task),
    },
  }
}

describe('waitTaskResultWithSse', () => {
  beforeEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.clearAllMocks()
    queryClient.clear()
    MockEventSource.instances = []
  })

  it('falls back to task polling when EventSource is unavailable', async () => {
    vi.stubGlobal('EventSource', undefined)
    mocks.getTask.mockResolvedValueOnce(
      makeTaskResponse('completed', {
        outputs: [makeImageOutput(' https://cdn.example.com/image.png ')],
      }),
    )

    await expect(waitTaskResultWithSse('task-1')).resolves.toEqual([
      'https://cdn.example.com/image.png',
    ])
    expect(mocks.getTask).toHaveBeenCalledWith('task-1')
  })

  it('falls back to polling with backoff after an EventSource error', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('EventSource', MockEventSource)
    mocks.getTask.mockResolvedValueOnce(makeTaskResponse('running')).mockResolvedValueOnce(
      makeTaskResponse('completed', {
        result: JSON.stringify({ image_url: 'https://cdn.example.com/done.png' }),
      }),
    )

    const result = waitTaskResultWithSse('task-1', {
      timeoutMs: 10_000,
      pollInitialDelayMs: 50,
      pollMaxDelayMs: 50,
    })
    MockEventSource.instances[0]?.fail()

    await vi.advanceTimersByTimeAsync(50)

    await expect(result).resolves.toEqual(['https://cdn.example.com/done.png'])
    expect(mocks.getTask).toHaveBeenCalledTimes(2)
    expect(MockEventSource.instances[0]?.closed).toBe(true)
  })

  it('falls back to polling when the backend sends a close event', async () => {
    vi.stubGlobal('EventSource', MockEventSource)
    mocks.getTask.mockResolvedValueOnce(
      makeTaskResponse('completed', {
        outputs: [makeImageOutput('https://cdn.example.com/after-close.png')],
      }),
    )

    const result = waitTaskResultWithSse('task-1')
    MockEventSource.instances[0]?.emit('close', { reason: 'max_connection_duration_reached' })

    await expect(result).resolves.toEqual(['https://cdn.example.com/after-close.png'])
    expect(mocks.getTask).toHaveBeenCalledTimes(1)
  })

  it('rejects explicitly when a task is cancelled from the SSE stream', async () => {
    vi.stubGlobal('EventSource', MockEventSource)

    const result = waitTaskResultWithSse('task-1')
    MockEventSource.instances[0]?.emit('task', {
      task_id: 'task-1',
      status: 'cancelled',
      cancel_reason: '用户取消',
    })

    await expect(result).rejects.toThrow('用户取消')
    expect(mocks.getTask).not.toHaveBeenCalled()
  })

  it('rejects explicitly when polling observes a cancelled task', async () => {
    vi.stubGlobal('EventSource', undefined)
    mocks.getTask.mockResolvedValueOnce(
      makeTaskResponse('cancelled', {
        cancel_reason: '账号已停用',
      }),
    )

    await expect(waitTaskResultWithSse('task-1')).rejects.toThrow('账号已停用')
  })
})
