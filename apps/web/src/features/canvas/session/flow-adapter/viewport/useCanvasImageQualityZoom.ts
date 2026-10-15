import { onScopeDispose, readonly, ref, watch, type Ref } from 'vue'

export type CanvasImageQuality = {
  zoomLevel: Readonly<Ref<number>>
  isSettled: Readonly<Ref<boolean>>
  requestSettle: () => void
}

const IMAGE_QUALITY_SETTLE_DELAY_MS = 80

export function useCanvasImageQualityZoom(
  zoomLevel: Readonly<Ref<number>>,
  getScopeKey: () => string,
) {
  const qualityZoomLevel = ref(zoomLevel.value)
  const isSettled = ref(true)
  const movements = new Set<symbol>()
  let settleTimer: ReturnType<typeof setTimeout> | undefined
  let isDisposed = false

  function clearSettleTimer() {
    clearTimeout(settleTimer)
    settleTimer = undefined
  }

  function scheduleSettle() {
    clearSettleTimer()
    if (isDisposed) return
    isSettled.value = false
    if (movements.size > 0) return
    settleTimer = setTimeout(() => {
      settleTimer = undefined
      qualityZoomLevel.value = zoomLevel.value
      isSettled.value = true
    }, IMAGE_QUALITY_SETTLE_DELAY_MS)
  }

  function beginMovement() {
    clearSettleTimer()
    isSettled.value = false
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
      isSettled.value = true
    },
    { flush: 'sync' },
  )
  onScopeDispose(() => {
    isDisposed = true
    clearSettleTimer()
    movements.clear()
  })

  const imageQuality: CanvasImageQuality = {
    zoomLevel: readonly(qualityZoomLevel),
    isSettled: readonly(isSettled),
    requestSettle: scheduleSettle,
  }
  return { imageQuality, beginMovement }
}
