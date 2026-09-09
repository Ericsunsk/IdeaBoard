import { onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router'

type UseCanvasLeaveGuardOptions = {
  onLeave?: () => void | Promise<void>
}

export function useCanvasLeaveGuard(options: UseCanvasLeaveGuardOptions = {}) {
  onBeforeRouteUpdate(async () => {
    await options.onLeave?.()
    return true
  })

  onBeforeRouteLeave(async () => {
    await options.onLeave?.()
    return true
  })
}
