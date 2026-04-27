import { computed, onMounted, onUnmounted, ref, watch, type Ref } from 'vue'

export interface CanvasRect {
  left: number
  top: number
  right: number
  bottom: number
}

export interface CanvasRectLike {
  x: number
  y: number
  width: number
  height: number
}

interface UseCanvasViewportCullingOptions {
  canvasLayoutRef: Ref<HTMLElement | null>
  canvasX: Ref<number>
  canvasY: Ref<number>
  zoomLevel: Ref<number>
  buffer?: Ref<number> | number
  onViewportRectChanged?: () => void
}

function toFiniteNumber(value: number, fallback: number): number {
  return Number.isFinite(value) ? value : fallback
}

function normalizeRect(rect: CanvasRectLike): CanvasRectLike {
  const width = Math.max(1, toFiniteNumber(rect.width, 1))
  const height = Math.max(1, toFiniteNumber(rect.height, 1))
  return {
    x: toFiniteNumber(rect.x, 0),
    y: toFiniteNumber(rect.y, 0),
    width,
    height,
  }
}

export function isRectVisible(rect: CanvasRectLike, visibleRect: CanvasRect | null): boolean {
  if (!visibleRect) return true
  const normalized = normalizeRect(rect)
  return (
    normalized.x < visibleRect.right &&
    normalized.x + normalized.width > visibleRect.left &&
    normalized.y < visibleRect.bottom &&
    normalized.y + normalized.height > visibleRect.top
  )
}

export function useCanvasViewportCulling(options: UseCanvasViewportCullingOptions) {
  const layoutSizeVersion = ref(0)
  let resizeObserver: ResizeObserver | null = null
  let observedElement: HTMLElement | null = null
  let windowResizeBound = false

  const bumpLayoutSizeVersion = () => {
    layoutSizeVersion.value += 1
    options.onViewportRectChanged?.()
  }

  const onWindowResize = () => {
    bumpLayoutSizeVersion()
  }

  const stopObservingLayout = () => {
    if (resizeObserver && observedElement) {
      resizeObserver.unobserve(observedElement)
    }
    observedElement = null
  }

  const startObservingLayout = (element: HTMLElement | null) => {
    stopObservingLayout()
    observedElement = element
    if (!element) return
    if (resizeObserver) {
      resizeObserver.observe(element)
    }
    bumpLayoutSizeVersion()
  }

  onMounted(() => {
    if (typeof window === 'undefined') return
    if (typeof ResizeObserver !== 'undefined') {
      resizeObserver = new ResizeObserver(() => {
        bumpLayoutSizeVersion()
      })
    } else {
      window.addEventListener('resize', onWindowResize)
      windowResizeBound = true
    }
    startObservingLayout(options.canvasLayoutRef.value)
  })

  onUnmounted(() => {
    stopObservingLayout()
    if (resizeObserver) {
      resizeObserver.disconnect()
      resizeObserver = null
    }
    if (windowResizeBound && typeof window !== 'undefined') {
      window.removeEventListener('resize', onWindowResize)
      windowResizeBound = false
    }
  })

  watch(options.canvasLayoutRef, (element) => {
    startObservingLayout(element)
  })

  const visibleCanvasRect = computed<CanvasRect | null>(() => {
    layoutSizeVersion.value
    const layout = options.canvasLayoutRef.value
    const layoutWidth = layout?.clientWidth ?? 0
    const layoutHeight = layout?.clientHeight ?? 0
    if (layoutWidth <= 0 || layoutHeight <= 0) return null

    const zoomScale = Math.max(0.01, options.zoomLevel.value / 100)
    const centerX = -options.canvasX.value / zoomScale
    const centerY = -options.canvasY.value / zoomScale
    const halfWidth = layoutWidth / (2 * zoomScale)
    const halfHeight = layoutHeight / (2 * zoomScale)

    const rawBuffer =
      typeof options.buffer === 'number'
        ? options.buffer
        : options.buffer?.value
    const buffer = Math.max(0, toFiniteNumber(rawBuffer ?? 1000, 1000))

    return {
      left: centerX - halfWidth - buffer,
      right: centerX + halfWidth + buffer,
      top: centerY - halfHeight - buffer,
      bottom: centerY + halfHeight + buffer,
    }
  })

  function isNodeRectVisible(rect: CanvasRectLike): boolean {
    return isRectVisible(rect, visibleCanvasRect.value)
  }

  function filterVisible<T>(items: readonly T[], resolveRect: (item: T) => CanvasRectLike): T[] {
    const rect = visibleCanvasRect.value
    if (!rect) return [...items]
    return items.filter(item => isRectVisible(resolveRect(item), rect))
  }

  return {
    visibleCanvasRect,
    isNodeRectVisible,
    filterVisible,
  }
}
