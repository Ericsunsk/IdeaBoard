import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { createApp, defineComponent, nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useCreditRunDetail } from './useCreditRunDetail'

async function flushQuery() {
  for (let i = 0; i < 5; i++) {
    await Promise.resolve()
    await nextTick()
  }
}

type TestRun = { id: number; status: string }

function mountDetail(load: (runId: number, signal: AbortSignal) => Promise<TestRun>) {
  const queryClient = new QueryClient()
  let detail!: ReturnType<typeof useCreditRunDetail<TestRun>>
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        detail = useCreditRunDetail(load)
        return () => null
      },
    }),
  )
  app.use(VueQueryPlugin, { queryClient })
  app.mount(host)
  return {
    detail,
    cleanup() {
      app.unmount()
      queryClient.clear()
      host.remove()
    },
  }
}

describe('useCreditRunDetail', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('polls queued or running Run detail every three seconds and stops at a terminal status', async () => {
    vi.useFakeTimers()
    const load = vi
      .fn()
      .mockResolvedValueOnce({ id: 21, status: 'running' })
      .mockResolvedValueOnce({ id: 21, status: 'completed' })
    const { detail, cleanup } = mountDetail(load)

    await detail.open(21)
    await flushQuery()
    expect(detail.run.value).toEqual({ id: 21, status: 'running' })

    await vi.advanceTimersByTimeAsync(3_000)
    expect(load).toHaveBeenCalledTimes(2)
    expect(detail.run.value).toEqual({ id: 21, status: 'completed' })

    await vi.advanceTimersByTimeAsync(6_000)
    expect(load).toHaveBeenCalledTimes(2)
    cleanup()
  })

  it('cancels the in-flight request and clears state after the detail closes', async () => {
    vi.useFakeTimers()
    let resolveRequest: (run: TestRun) => void = () => undefined
    const load = vi.fn(
      (_runId: number, signal: AbortSignal) =>
        new Promise<TestRun>((resolve) => {
          resolveRequest = resolve
          expect(signal.aborted).toBe(false)
        }),
    )
    const { detail, cleanup } = mountDetail(load)

    void detail.open(21)
    await flushQuery()
    detail.close()
    resolveRequest({ id: 21, status: 'running' })
    await flushQuery()

    expect(load.mock.calls[0]?.[1].aborted).toBe(true)
    expect(detail.run.value).toBeUndefined()
    expect(detail.loading.value).toBe(false)
    await vi.advanceTimersByTimeAsync(3_000)
    expect(load).toHaveBeenCalledTimes(1)
    cleanup()
  })
})
