import type { Ref } from 'vue'
import type { CanvasImageItem, ModelNodeItem } from '../types/canvasNodes'

interface UploadResponse {
  code: number
  msg?: string
  data?: {
    url?: string
  }
}

interface ModelNodeMessageApi {
  warning(content: string): unknown
  success(content: string): unknown
  error(content: string): unknown
}

interface DragPayload {
  url: string
  ratio: number
}

interface UseModelNodeOptions {
  modelNodes: Ref<ModelNodeItem[]>
  message: ModelNodeMessageApi
  uploadImage: (file: File) => Promise<UploadResponse>
  loadImageRatioFromFile: (file: File) => Promise<number | null>
  loadImageRatioFromUrl: (url: string) => Promise<number | null>
  readNodeImageDropPayload: (event: DragEvent) => DragPayload | null
}

export function useModelNode(options: UseModelNodeOptions) {
  async function syncModelNodeImageRatioByUrl(node: ModelNodeItem, url: string) {
    const ratio = await options.loadImageRatioFromUrl(url)
    if (!ratio || node.sourceImageUrl !== url) return
    node.sourceImageRatio = ratio
  }

  function getModelStageHeight(node: ModelNodeItem): number {
    if (!node.sourceImageUrl) return 176
    const stageWidth = Math.max(120, node.width - 24)
    const ratio = Number.isFinite(node.sourceImageRatio) && node.sourceImageRatio > 0
      ? node.sourceImageRatio
      : 1
    const dynamicHeight = Math.round(stageWidth * ratio)
    return Math.max(120, Math.min(420, dynamicHeight))
  }

  function toggleModelAngle(node: ModelNodeItem, angleKey: string) {
    const idx = node.selectedAngles.indexOf(angleKey)
    if (idx >= 0) {
      node.selectedAngles.splice(idx, 1)
      return
    }
    if (node.selectedAngles.length >= 4) {
      options.message.warning('最多选择 4 个角度')
      return
    }
    node.selectedAngles.push(angleKey)
  }

  function clearModelAngles(node: ModelNodeItem) {
    node.selectedAngles = []
  }

  async function uploadModelNodeFile(node: ModelNodeItem, file: File) {
    if (!file.type.startsWith('image/')) {
      options.message.warning('请上传图片文件')
      return
    }
    try {
      const ratioFromFile = await options.loadImageRatioFromFile(file)
      const res = await options.uploadImage(file)
      if (res.code === 0 && res.data?.url) {
        node.sourceImageUrl = res.data.url
        if (ratioFromFile) node.sourceImageRatio = ratioFromFile
        else void syncModelNodeImageRatioByUrl(node, res.data.url)
        options.message.success('节点图片已更新')
        return
      }
      options.message.error(res.msg || '上传失败')
    } catch (error: unknown) {
      const maybeError = error as { msg?: string; message?: string } | null
      options.message.error(maybeError?.msg || maybeError?.message || '上传失败')
    }
  }

  function openModelNodeFilePicker(node: ModelNodeItem) {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = 'image/*'
    input.onchange = () => {
      const file = input.files?.[0]
      if (!file) return
      void uploadModelNodeFile(node, file)
    }
    input.click()
  }

  function clearModelNodeImage(node: ModelNodeItem) {
    node.sourceImageUrl = ''
    node.sourceImageRatio = 1
  }

  function onModelStageDrop(event: DragEvent, node: ModelNodeItem) {
    const payload = options.readNodeImageDropPayload(event)
    if (payload) {
      node.sourceImageUrl = payload.url
      if (payload.ratio > 0) node.sourceImageRatio = payload.ratio
      else void syncModelNodeImageRatioByUrl(node, payload.url)
      return
    }
    const file = event.dataTransfer?.files?.[0]
    if (!file) return
    void uploadModelNodeFile(node, file)
  }

  function importCanvasImageToModelNodeAtPoint(clientX: number, clientY: number, item: CanvasImageItem): boolean {
    const target = document.elementFromPoint(clientX, clientY) as HTMLElement | null
    const stageEl = target?.closest('.model-stage[data-model-id]') as HTMLElement | null
    if (!stageEl) return false
    const modelId = Number(stageEl.dataset.modelId)
    if (!Number.isFinite(modelId)) return false
    const node = options.modelNodes.value.find(n => n.id === modelId)
    if (!node) return false
    node.sourceImageUrl = item.url
    if (item.height_ratio && item.height_ratio > 0) node.sourceImageRatio = item.height_ratio
    else void syncModelNodeImageRatioByUrl(node, item.url)
    return true
  }

  return {
    getModelStageHeight,
    toggleModelAngle,
    clearModelAngles,
    openModelNodeFilePicker,
    clearModelNodeImage,
    onModelStageDrop,
    importCanvasImageToModelNodeAtPoint,
  }
}
