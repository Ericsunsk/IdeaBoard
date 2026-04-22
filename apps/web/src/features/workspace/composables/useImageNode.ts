import type { Ref } from 'vue'
import type {
  CanvasImageItem,
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
  message: ImageNodeMessageApi
  uploadImage: (file: File) => Promise<UploadResponse>
  loadImageRatioFromFile: (file: File) => Promise<number | null>
  loadImageRatioFromUrl: (url: string) => Promise<number | null>
  readNodeImageDropPayload: (event: DragEvent) => DragPayload | null
  hasIncomingImageConnection?: (nodeId: number) => boolean
}

export function useImageNode(options: UseImageNodeOptions) {
  function hasLockedIncomingImage(node: ImageNodeItem) {
    return options.hasIncomingImageConnection?.(node.id) ?? false
  }

  function warnLockedIncomingImage() {
    options.message.warning('已连接上游 Image 输入，请先断开后再替换本地图')
  }

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
    if (hasLockedIncomingImage(node)) {
      warnLockedIncomingImage()
      return
    }
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
    if (hasLockedIncomingImage(node)) {
      warnLockedIncomingImage()
      return
    }
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
    if (hasLockedIncomingImage(node)) {
      warnLockedIncomingImage()
      return
    }
    clearImageNodeSource(node)
    node.outputImages = []
  }

  function onImageNodeStageDrop(event: DragEvent, node: ImageNodeItem) {
    if (hasLockedIncomingImage(node)) {
      warnLockedIncomingImage()
      return
    }
    const payload = options.readNodeImageDropPayload(event)
    if (payload) {
      setImageNodeInput(node, payload.url, payload.ratio, true)
      return
    }
    const file = event.dataTransfer?.files?.[0]
    if (!file) return
    void uploadImageNodeFile(node, file)
  }

  function importCanvasImageToImageNodeAtPoint(clientX: number, clientY: number, item: CanvasImageItem): boolean {
    const target = document.elementFromPoint(clientX, clientY) as HTMLElement | null
    const stageEl = target?.closest('.image-node-stage[data-image-node-id]') as HTMLElement | null
    if (!stageEl) return false
    const imageNodeId = Number(stageEl.dataset.imageNodeId)
    if (!Number.isFinite(imageNodeId)) return false
    const node = options.imageNodes.value.find(n => n.id === imageNodeId)
    if (!node) return false
    if (hasLockedIncomingImage(node)) {
      warnLockedIncomingImage()
      return false
    }
    setImageNodeInput(node, item.url, item.height_ratio, true)
    return true
  }

  return {
    getImageNodePreferredImage,
    setImageNodeInput,
    openImageNodeFilePicker,
    resetImageNode,
    onImageNodeStageDrop,
    importCanvasImageToImageNodeAtPoint,
  }
}
