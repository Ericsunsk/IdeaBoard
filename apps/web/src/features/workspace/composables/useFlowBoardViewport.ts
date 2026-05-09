import { ref } from 'vue'
import { useVueFlow } from '@vue-flow/core'

export function useFlowBoardViewport() {
  const { zoomIn, zoomOut, getViewport, setViewport } = useVueFlow()

  const zoomLevel = ref(100)
  let lastFlowZoom = 100

  /** Apply a Vue Flow transform payload. Returns true if zoom changed. */
  function applyFlowTransform(transform: { x?: number; y?: number; zoom?: number }): boolean {
    let zoomChanged = false
    if (Number.isFinite(transform.zoom)) {
      const nextZoom = Math.max(10, Math.round((transform.zoom as number) * 100))
      if (lastFlowZoom !== nextZoom) {
        zoomChanged = true
      }
      zoomLevel.value = nextZoom
      lastFlowZoom = nextZoom
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
    onZoomIn,
    onZoomOut,
    getViewport,
    setViewport,
    applyFlowTransform,
  }
}
