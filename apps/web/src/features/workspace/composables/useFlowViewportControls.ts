import { ref } from 'vue'

type FlowTransform = { x?: number; y?: number; zoom?: number }
type PanDelta = { x: number; y: number }
type FlowViewport = { x: number; y: number; zoom: number }

type UseFlowViewportControlsParams = {
  zoomIn: () => void
  zoomOut: () => void
  getViewport: () => FlowViewport
  setViewport: (viewport: FlowViewport) => unknown
}

export function useFlowViewportControls({
  zoomIn,
  zoomOut,
  getViewport,
  setViewport,
}: UseFlowViewportControlsParams) {
  const zoomLevel = ref(100)
  let lastFlowZoom = 100

  function onZoomIn() {
    zoomIn()
  }

  function onZoomOut() {
    zoomOut()
  }

  function applyFlowTransform(transform: FlowTransform): boolean {
    let zoomChanged = false
    if (Number.isFinite(transform.zoom)) {
      const nextZoom = Math.max(10, Math.round((transform.zoom as number) * 100))
      if (lastFlowZoom !== nextZoom) zoomChanged = true
      zoomLevel.value = nextZoom
      lastFlowZoom = nextZoom
    }
    return zoomChanged
  }

  function onPanViewport(delta: PanDelta) {
    const viewport = getViewport()
    setViewport({
      x: viewport.x + delta.x,
      y: viewport.y + delta.y,
      zoom: viewport.zoom,
    })
  }

  return {
    zoomLevel,
    onZoomIn,
    onZoomOut,
    applyFlowTransform,
    onPanViewport,
  }
}
