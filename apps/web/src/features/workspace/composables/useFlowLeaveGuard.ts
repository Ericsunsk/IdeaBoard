import { onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router'

type UseFlowLeaveGuardOptions = {
  onLeave?: () => void | Promise<void>
}

export function useFlowLeaveGuard(options: UseFlowLeaveGuardOptions = {}) {
  onBeforeRouteUpdate(async () => {
    await options.onLeave?.()
    return true
  })

  onBeforeRouteLeave(async () => {
    await options.onLeave?.()
    return true
  })
}
