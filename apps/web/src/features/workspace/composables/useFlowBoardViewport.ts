import { computed, ref } from 'vue'
import { useVueFlow } from '@vue-flow/core'

export function useFlowBoardViewport() {
  const { zoomIn, zoomOut, getViewport, setViewport } = useVueFlow()

  const zoomLevel = ref(100)
  const viewportOffsetX = ref(0)
  const viewportOffsetY = ref(0)
  let lastFlowZoom: number | null = null

  const canvasGridStyle = computed(() => ({
    '--zoom': `${Math.max(0.5, zoomLevel.value / 100)}`,
    '--offset-x': `${viewportOffsetX.value}px`,
    '--offset-y': `${viewportOffsetY.value}px`,
  }))

  /** Apply a Vue Flow transform payload. Returns true if zoom changed. */
  function applyFlowTransform(transform: { x?: number; y?: number; zoom?: number }): boolean {
    let zoomChanged = false
    if (Number.isFinite(transform.zoom)) {
      const nextZoom = Math.max(10, Math.round((transform.zoom as number) * 100))
      if (lastFlowZoom !== null && lastFlowZoom !== nextZoom) {
        zoomChanged = true
      }
      zoomLevel.value = nextZoom
      lastFlowZoom = nextZoom
    }
    if (Number.isFinite(transform.x)) {
      viewportOffsetX.value = transform.x as number
    }
    if (Number.isFinite(transform.y)) {
      viewportOffsetY.value = transform.y as number
    }
    return zoomChanged
  }

  function onZoomIn() {
    zoomIn()
  }

  function onZoomOut() {
    zoomOut()
  }

  return {
    zoomLevel,
    viewportOffsetX,
    viewportOffsetY,
    canvasGridStyle,
    onZoomIn,
    onZoomOut,
    getViewport,
    setViewport,
    applyFlowTransform,
  }
}
