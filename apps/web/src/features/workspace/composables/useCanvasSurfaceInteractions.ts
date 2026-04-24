import { onMounted, onUnmounted, type Ref } from 'vue'
import type { CanvasImageItem, CanvasPoint } from '../types/canvasNodes'

interface UseCanvasSurfaceInteractionsOptions {
  messages: Ref<CanvasImageItem[]>
  selectedIds: Ref<number[]>
  canvasX: Ref<number>
  canvasY: Ref<number>
  activeImageActionId: Ref<number | null>
  nodeImageDragMime: string
  closeNodeContextMenu: () => void
  updateZoom: (delta: number, anchor?: { clientX: number; clientY: number }) => void
  getCanvasPointFromClient: (clientX: number, clientY: number) => CanvasPoint
  createCanvasImageAt: (point: CanvasPoint, url: string, ratio?: number) => CanvasImageItem
  uploadImage: (file: File) => Promise<{ code?: number; data?: { url?: string } }>
  resolveCanvasImageUploadMeta?: (url: string) => CanvasImageItem['uploadMeta'] | undefined
}

export function useCanvasSurfaceInteractions(options: UseCanvasSurfaceInteractionsOptions) {
  function readNodeImageDropPayload(e: DragEvent): { url: string; ratio: number } | null {
    const rawUrl = e.dataTransfer?.getData(options.nodeImageDragMime)
    if (!rawUrl) return null
    const url = rawUrl.trim()
    if (!url) return null
    const rawRatio = e.dataTransfer?.getData(`${options.nodeImageDragMime}-ratio`) || ''
    const ratio = Number(rawRatio)
    return {
      url,
      ratio: Number.isFinite(ratio) && ratio > 0 ? ratio : 1,
    }
  }

  function handleWheel(e: WheelEvent) {
    const target = e.target as HTMLElement | null
    if (!target || !target.closest('.canvas-layout')) return
    const wheelPanSpeed = 1.5
    options.closeNodeContextMenu()

    if (target.closest('.settings-panel') || target.closest('.selected-preview-list')) return

    if (e.altKey) {
      e.preventDefault()
      options.updateZoom(e.deltaY > 0 ? -5 : 5, { clientX: e.clientX, clientY: e.clientY })
      return
    }

    if (e.ctrlKey || e.metaKey) {
      e.preventDefault()
      const horizontalDelta = (Math.abs(e.deltaX) > 0 ? e.deltaX : e.deltaY) * wheelPanSpeed
      options.canvasX.value -= horizontalDelta
      return
    }

    e.preventDefault()
    options.canvasX.value -= e.deltaX * wheelPanSpeed
    options.canvasY.value -= e.deltaY * wheelPanSpeed
  }

  async function handleDrop(e: DragEvent) {
    e.preventDefault()
    const payload = readNodeImageDropPayload(e)
    if (payload) {
      const point = options.getCanvasPointFromClient(e.clientX, e.clientY)
      const newItem = options.createCanvasImageAt(point, payload.url, payload.ratio)
      options.messages.value.push(newItem)
      options.selectedIds.value = [newItem.id]
      options.activeImageActionId.value = null
      return
    }

    const files = e.dataTransfer?.files
    if (!files || !files[0]) return
    const { x, y } = options.getCanvasPointFromClient(e.clientX, e.clientY)

    for (const file of Array.from(files)) {
      if (!file.type.startsWith('image/')) continue
      try {
        const res = await options.uploadImage(file)
        if (res.code === 0 && res.data?.url) {
          const item = options.createCanvasImageAt({ x, y }, res.data.url)
          const uploadMeta = options.resolveCanvasImageUploadMeta?.(res.data.url)
          if (uploadMeta) {
            item.uploadMeta = uploadMeta
          }
          options.messages.value.push(item)
        }
      } catch {
        // Keep drop interaction resilient; upload failure is already surfaced elsewhere.
      }
    }
  }

  onMounted(() => {
    window.addEventListener('wheel', handleWheel, { passive: false })
  })

  onUnmounted(() => {
    window.removeEventListener('wheel', handleWheel)
  })

  return {
    readNodeImageDropPayload,
    handleDrop,
  }
}
