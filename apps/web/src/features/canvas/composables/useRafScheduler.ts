import { tryOnScopeDispose } from '@vueuse/core'

export function useRafScheduler() {
  let frameId: number | null = null
  let timeoutId: ReturnType<typeof setTimeout> | null = null
  let isDisposed = false
  let scheduleToken = 0

  function cancel() {
    scheduleToken += 1
    if (frameId !== null && typeof cancelAnimationFrame === 'function') {
      cancelAnimationFrame(frameId)
    }
    if (timeoutId !== null) {
      clearTimeout(timeoutId)
    }
    frameId = null
    timeoutId = null
  }

  function schedule(callback: () => void) {
    if (isDisposed) return
    cancel()
    const activeToken = scheduleToken

    const run = () => {
      frameId = null
      timeoutId = null
      if (!isDisposed && activeToken === scheduleToken) callback()
    }

    if (typeof requestAnimationFrame === 'function') {
      frameId = requestAnimationFrame(run)
      return
    }

    timeoutId = setTimeout(run, 0)
  }

  tryOnScopeDispose(() => {
    isDisposed = true
    cancel()
  })

  return {
    schedule,
    cancel,
  }
}
