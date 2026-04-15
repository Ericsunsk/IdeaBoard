import type { Ref } from 'vue'
import type {
  CanvasImageItem,
  CanvasPoint,
  ImageNodeItem,
  ImageNodeOutputItem,
} from '../types/canvasNodes'

interface UploadResponse {
  code: number
  msg?: string
  data?: {
    url?: string
  }
}

interface ImageNodeMessageApi {
  warning(content: string): unknown
  success(content: string): unknown
  error(content: string): unknown
}

interface DragPayload {
  url: string
  ratio: number
}

interface UseImageNodeOptions {
  imageNodes: Ref<ImageNodeItem[]>
  messages: Ref<CanvasImageItem[]>
  selectedIds: Ref<number[]>
  message: ImageNodeMessageApi
  uploadImage: (file: File) => Promise<UploadResponse>
  loadImageRatioFromFile: (file: File) => Promise<number | null>
  loadImageRatioFromUrl: (url: string) => Promise<number | null>
  readNodeImageDropPayload: (event: DragEvent) => DragPayload | null
  createCanvasImageAt: (point: CanvasPoint, url: string, ratio?: number) => CanvasImageItem
  nodeImageDragMime: string
}

export function useImageNode(options: UseImageNodeOptions) {
  async function syncImageNodeImageRatioByUrl(node: ImageNodeItem, url: string) {
    const ratio = await options.loadImageRatioFromUrl(url)
    if (!ratio || node.sourceImageUrl !== url) return
    node.sourceImageRatio = ratio
  }

  function getImageNodePreferredImage(node: ImageNodeItem | null): ImageNodeOutputItem | null {
    if (!node) return null
    if (node.outputImages.length > 0) {
      const latestOutput = node.outputImages[node.outputImages.length - 1]
      return {
        url: latestOutput.url,
        ratio: latestOutput.ratio > 0 ? latestOutput.ratio : 1,
      }
    }
    if (node.sourceImageUrl) {
      return {
        url: node.sourceImageUrl,
        ratio: node.sourceImageRatio > 0 ? node.sourceImageRatio : 1,
      }
    }
    return null
  }

  function getImageNodeDisplayImage(node: ImageNodeItem): ImageNodeOutputItem | null {
    return getImageNodePreferredImage(node)
  }

  function getImageNodeStageHeight(node: ImageNodeItem): number {
    const displayImage = getImageNodePreferredImage(node)
    if (!displayImage) return 160
    const stageWidth = Math.max(120, node.width - 24)
    const ratio = Number.isFinite(displayImage.ratio) && displayImage.ratio > 0
      ? displayImage.ratio
      : 1
    const dynamicHeight = Math.round(stageWidth * ratio)
    return Math.max(110, dynamicHeight)
  }

  function setImageNodeInput(node: ImageNodeItem, url: string, ratio?: number | null, clearOutputs = true) {
    node.sourceImageUrl = url
    if (ratio && ratio > 0) {
      node.sourceImageRatio = ratio
    } else {
      void syncImageNodeImageRatioByUrl(node, url)
    }
    if (clearOutputs) {
      node.outputImages = []
    }
  }

  async function uploadImageNodeFile(node: ImageNodeItem, file: File) {
    if (!file.type.startsWith('image/')) {
      options.message.warning('请上传图片文件')
      return
    }
    try {
      const ratioFromFile = await options.loadImageRatioFromFile(file)
      const res = await options.uploadImage(file)
      if (res.code === 0 && res.data?.url) {
        setImageNodeInput(node, res.data.url, ratioFromFile, true)
        options.message.success('Image 节点输入已更新')
        return
      }
      options.message.error(res.msg || '上传失败')
    } catch (error: unknown) {
      const maybeError = error as { msg?: string; message?: string } | null
      options.message.error(maybeError?.msg || maybeError?.message || '上传失败')
    }
  }

  function openImageNodeFilePicker(node: ImageNodeItem) {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = 'image/*'
    input.onchange = () => {
      const file = input.files?.[0]
      if (!file) return
      void uploadImageNodeFile(node, file)
    }
    input.click()
  }

  function clearImageNodeSource(node: ImageNodeItem) {
    node.sourceImageUrl = ''
    node.sourceImageRatio = 1
  }

  function resetImageNode(node: ImageNodeItem) {
    clearImageNodeSource(node)
    node.outputImages = []
  }

  function onImageNodeStageDrop(event: DragEvent, node: ImageNodeItem) {
    const payload = options.readNodeImageDropPayload(event)
    if (payload) {
      setImageNodeInput(node, payload.url, payload.ratio, true)
      return
    }
    const file = event.dataTransfer?.files?.[0]
    if (!file) return
    void uploadImageNodeFile(node, file)
  }

  function appendImageNodeDisplayToCanvas(node: ImageNodeItem) {
    const display = getImageNodeDisplayImage(node)
    if (!display) return
    const item = options.createCanvasImageAt(
      { x: node.x - 360, y: node.y + 18 },
      display.url,
      display.ratio,
    )
    options.messages.value.push(item)
    options.selectedIds.value = [item.id]
  }

  function onImageNodeDisplayDragStart(event: DragEvent, node: ImageNodeItem) {
    const display = getImageNodeDisplayImage(node)
    if (!display || !event.dataTransfer) return
    event.dataTransfer.effectAllowed = 'copy'
    event.dataTransfer.setData(options.nodeImageDragMime, display.url)
    event.dataTransfer.setData(`${options.nodeImageDragMime}-ratio`, String(display.ratio || 1))
  }

  function importCanvasImageToImageNodeAtPoint(clientX: number, clientY: number, item: CanvasImageItem): boolean {
    const target = document.elementFromPoint(clientX, clientY) as HTMLElement | null
    const stageEl = target?.closest('.image-node-stage[data-image-node-id]') as HTMLElement | null
    if (!stageEl) return false
    const imageNodeId = Number(stageEl.dataset.imageNodeId)
    if (!Number.isFinite(imageNodeId)) return false
    const node = options.imageNodes.value.find(n => n.id === imageNodeId)
    if (!node) return false
    setImageNodeInput(node, item.url, item.height_ratio, true)
    return true
  }

  return {
    getImageNodeStageHeight,
    getImageNodePreferredImage,
    getImageNodeDisplayImage,
    setImageNodeInput,
    openImageNodeFilePicker,
    resetImageNode,
    onImageNodeStageDrop,
    appendImageNodeDisplayToCanvas,
    onImageNodeDisplayDragStart,
    importCanvasImageToImageNodeAtPoint,
  }
}
