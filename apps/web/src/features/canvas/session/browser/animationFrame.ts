import { nextTick } from 'vue'

export function waitForNextAnimationFrame(): Promise<void> {
  if (typeof requestAnimationFrame !== 'function') return Promise.resolve()
  return new Promise((resolve) => {
    requestAnimationFrame(() => resolve())
  })
}

export function waitForNextRenderFrame(): Promise<void> {
  if (typeof window === 'undefined') return nextTick()
  return nextTick().then(waitForNextAnimationFrame)
}
