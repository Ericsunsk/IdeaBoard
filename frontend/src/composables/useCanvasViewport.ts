import { computed, ref } from 'vue'
import type { CanvasPoint, CanvasViewportState } from '../types/canvasNodes'

const MIN_ZOOM_LEVEL = 10
const MAX_ZOOM_LEVEL = 200
const DEFAULT_ZOOM_LEVEL = 100

function clampZoomLevel(value: number): number {
  return Math.max(MIN_ZOOM_LEVEL, Math.min(MAX_ZOOM_LEVEL, Math.round(value)))
}

export function useCanvasViewport() {
  const zoomLevel = ref(DEFAULT_ZOOM_LEVEL)
  const canvasX = ref(0)
  const canvasY = ref(0)
  const canvasLayoutRef = ref<HTMLElement | null>(null)

  function setViewport(viewport: Partial<CanvasViewportState>) {
    canvasX.value = Number.isFinite(viewport.canvasX) ? Number(viewport.canvasX) : 0
    canvasY.value = Number.isFinite(viewport.canvasY) ? Number(viewport.canvasY) : 0
    zoomLevel.value = clampZoomLevel(
      Number.isFinite(viewport.zoomLevel) ? Number(viewport.zoomLevel) : DEFAULT_ZOOM_LEVEL,
    )
  }

  function resetViewport() {
    setViewport({
      canvasX: 0,
      canvasY: 0,
      zoomLevel: DEFAULT_ZOOM_LEVEL,
    })
  }

  function updateZoom(delta: number, anchor?: { clientX: number; clientY: number }) {
    const prevZoom = zoomLevel.value
    const nextZoom = clampZoomLevel(prevZoom + delta)
    if (nextZoom === prevZoom) return

    if (anchor && canvasLayoutRef.value) {
      const rect = canvasLayoutRef.value.getBoundingClientRect()
      const dx = anchor.clientX - (rect.left + rect.width / 2)
      const dy = anchor.clientY - (rect.top + rect.height / 2)
      const ratio = nextZoom / prevZoom
      canvasX.value = canvasX.value * ratio + dx * (1 - ratio)
      canvasY.value = canvasY.value * ratio + dy * (1 - ratio)
    }

    zoomLevel.value = nextZoom
  }

  function getCanvasPointFromClient(clientX: number, clientY: number): CanvasPoint {
    const rect = canvasLayoutRef.value?.getBoundingClientRect()
    if (!rect) return { x: 0, y: 0 }
    const zoom = zoomLevel.value / 100
    return {
      x: (clientX - rect.left - rect.width / 2 - canvasX.value) / zoom,
      y: (clientY - rect.top - rect.height / 2 - canvasY.value) / zoom,
    }
  }

  function createViewportSnapshot(): CanvasViewportState {
    return {
      canvasX: canvasX.value,
      canvasY: canvasY.value,
      zoomLevel: zoomLevel.value,
    }
  }

  const canvasStyle = computed(() => ({
    '--zoom': zoomLevel.value / 100,
    '--offset-x': `${canvasX.value}px`,
    '--offset-y': `${canvasY.value}px`,
    '--grid-opacity': zoomLevel.value > 80 ? 1 : 0,
  }))

  return {
    zoomLevel,
    canvasX,
    canvasY,
    canvasLayoutRef,
    canvasStyle,
    updateZoom,
    getCanvasPointFromClient,
    setViewport,
    resetViewport,
    createViewportSnapshot,
  }
}
