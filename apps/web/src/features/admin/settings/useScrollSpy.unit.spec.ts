import { describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent } from 'vue'
import { useScrollSpy } from './useScrollSpy'

const SECTION_IDS = ['section-a', 'section-b', 'section-c'] as const

type ObserverCallback = (entries: IntersectionObserverEntry[]) => void

let capturedCallback: ObserverCallback | null = null

class FakeIntersectionObserver {
  callback: ObserverCallback

  constructor(callback: ObserverCallback) {
    this.callback = callback
    capturedCallback = callback
  }

  observe() {}

  disconnect() {}
}

function installObserverMock() {
  capturedCallback = null
  vi.stubGlobal('IntersectionObserver', FakeIntersectionObserver)
}

function fireIntersect(id: string) {
  const entry = {
    isIntersecting: true,
    target: { id },
    boundingClientRect: { top: 0 },
  } as unknown as IntersectionObserverEntry
  capturedCallback?.([entry])
}

function mountSpy(ids: readonly string[], options?: { lockMs?: number }) {
  let api: ReturnType<typeof useScrollSpy> | null = null
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        api = useScrollSpy(ids, options)
        return () => null
      },
    }),
  )
  app.mount(host)
  if (!api) throw new Error('expected scroll spy to mount')
  const mounted: ReturnType<typeof useScrollSpy> = api
  return {
    api: mounted,
    async unmount() {
      app.unmount()
      host.remove()
    },
  }
}

function mockScrollTo() {
  const fn = vi.fn()
  Object.defineProperty(window, 'scrollTo', { configurable: true, writable: true, value: fn })
  return fn as unknown as ReturnType<typeof vi.fn>
}

describe('useScrollSpy', () => {
  it('updates the highlight when scrolling reveals a new section', async () => {
    installObserverMock()
    const harness = mountSpy([...SECTION_IDS])
    try {
      expect(harness.api.activeId.value).toBe('section-a')
      fireIntersect('section-b')
      expect(harness.api.activeId.value).toBe('section-b')
    } finally {
      await harness.unmount()
      vi.unstubAllGlobals()
    }
  })

  it('ignores location.hash on mount', async () => {
    installObserverMock()
    const scrollTo = mockScrollTo()
    window.location.hash = '#section-c'
    const harness = mountSpy([...SECTION_IDS])
    try {
      expect(harness.api.activeId.value).toBe('section-a')
      expect(scrollTo).not.toHaveBeenCalled()
    } finally {
      await harness.unmount()
      window.location.hash = ''
      vi.unstubAllGlobals()
      vi.restoreAllMocks()
    }
  })

  it('unlocks on scrollend so later intersections update the highlight', async () => {
    installObserverMock()
    const harness = mountSpy([...SECTION_IDS], { lockMs: 10_000 })
    try {
      harness.api.handleSelect('section-b')
      expect(harness.api.activeId.value).toBe('section-b')
      // 锁定期间 observer 被忽略
      fireIntersect('section-c')
      expect(harness.api.activeId.value).toBe('section-b')
      // scrollend 解除锁定
      document.dispatchEvent(new Event('scrollend'))
      fireIntersect('section-c')
      expect(harness.api.activeId.value).toBe('section-c')
    } finally {
      await harness.unmount()
      vi.unstubAllGlobals()
    }
  })

  it('falls back to the lock timeout where scrollend never fires', async () => {
    installObserverMock()
    vi.useFakeTimers()
    const harness = mountSpy([...SECTION_IDS], { lockMs: 1000 })
    try {
      harness.api.handleSelect('section-b')
      fireIntersect('section-c')
      expect(harness.api.activeId.value).toBe('section-b')
      await vi.advanceTimersByTimeAsync(1000)
      fireIntersect('section-c')
      expect(harness.api.activeId.value).toBe('section-c')
    } finally {
      await harness.unmount()
      vi.unstubAllGlobals()
      vi.useRealTimers()
    }
  })

  it('removes the scrollend listener and clears timers on unmount', async () => {
    installObserverMock()
    const removeListener = vi.spyOn(document, 'removeEventListener')
    const harness = mountSpy([...SECTION_IDS])
    try {
      harness.api.handleSelect('section-b')
      await harness.unmount()
      expect(removeListener).toHaveBeenCalledWith('scrollend', expect.any(Function))
    } finally {
      removeListener.mockRestore()
      vi.unstubAllGlobals()
    }
  })
})
