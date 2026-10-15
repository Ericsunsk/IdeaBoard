import { onScopeDispose, readonly, ref, watch, type Ref } from 'vue'

const IMAGE_QUALITY_SETTLE_DELAY_MS = 200

export function useCanvasImageQualityZoom(
  zoomLevel: Readonly<Ref<number>>,
  getScopeKey: () => string,
) {
  const qualityZoomLevel = ref(zoomLevel.value)
  const movements = new Set<symbol>()
  let settleTimer: ReturnType<typeof setTimeout> | undefined
  let isDisposed = false

  function clearSettleTimer() {
    clearTimeout(settleTimer)
    settleTimer = undefined
  }

  function scheduleSettle() {
    clearSettleTimer()
    if (isDisposed || movements.size > 0 || qualityZoomLevel.value === zoomLevel.value) return
    settleTimer = setTimeout(() => {
      settleTimer = undefined
      qualityZoomLevel.value = zoomLevel.value
    }, IMAGE_QUALITY_SETTLE_DELAY_MS)
  }

  function beginMovement() {
    clearSettleTimer()
    const movement = Symbol()
    movements.add(movement)
    return () => {
      if (movements.delete(movement)) scheduleSettle()
    }
  }

  watch(zoomLevel, scheduleSettle, { flush: 'sync' })
  watch(
    getScopeKey,
    () => {
      clearSettleTimer()
      movements.clear()
      qualityZoomLevel.value = zoomLevel.value
    },
    { flush: 'sync' },
  )
  onScopeDispose(() => {
    isDisposed = true
    clearSettleTimer()
    movements.clear()
  })

  return { qualityZoomLevel: readonly(qualityZoomLevel), beginMovement }
}
