import { onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router'

type UseFlowLeaveGuardOptions = {
  onLeave?: () => void
}

export function useFlowLeaveGuard(options: UseFlowLeaveGuardOptions = {}) {
  onBeforeRouteUpdate(() => {
    options.onLeave?.()
    return true
  })

  onBeforeRouteLeave(() => {
    options.onLeave?.()
    return true
  })
}
