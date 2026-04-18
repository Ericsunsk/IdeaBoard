import type { Ref } from 'vue'
import type { CanvasImageItem, MannequinNodeItem } from '../types/canvasNodes'

interface UploadResponse {
  code: number
  msg?: string
  data?: {
    url?: string
  }
}

interface MannequinNodeMessageApi {
  warning(content: string): unknown
  success(content: string): unknown
  error(content: string): unknown
}

interface DragPayload {
  url: string
  ratio: number
}

interface UseMannequinNodeOptions {
  mannequinNodes: Ref<MannequinNodeItem[]>
  message: MannequinNodeMessageApi
  uploadImage: (file: File) => Promise<UploadResponse>
  loadImageRatioFromFile: (file: File) => Promise<number | null>
  loadImageRatioFromUrl: (url: string) => Promise<number | null>
  readNodeImageDropPayload: (event: DragEvent) => DragPayload | null
}

export function useMannequinNode(options: UseMannequinNodeOptions) {
  async function syncMannequinNodeImageRatioByUrl(node: MannequinNodeItem, url: string) {
    const ratio = await options.loadImageRatioFromUrl(url)
    if (!ratio || node.sourceImageUrl !== url) return
    node.sourceImageRatio = ratio
  }

  function getMannequinStageHeight(
    node: MannequinNodeItem,
    sourceImageUrl = node.sourceImageUrl,
    sourceImageRatio = node.sourceImageRatio,
  ): number {
    if (!sourceImageUrl) return 176
    const stageWidth = Math.max(120, node.width - 24)
    const ratio = Number.isFinite(sourceImageRatio) && sourceImageRatio > 0
      ? sourceImageRatio
      : 1
    const dynamicHeight = Math.round(stageWidth * ratio)
    return Math.max(120, Math.min(420, dynamicHeight))
  }

  async function uploadMannequinNodeFile(node: MannequinNodeItem, file: File) {
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
        else void syncMannequinNodeImageRatioByUrl(node, res.data.url)
        options.message.success('节点图片已更新')
        return
      }
      options.message.error(res.msg || '上传失败')
    } catch (error: unknown) {
      const maybeError = error as { msg?: string; message?: string } | null
      options.message.error(maybeError?.msg || maybeError?.message || '上传失败')
    }
  }

  function openMannequinNodeFilePicker(node: MannequinNodeItem) {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = 'image/*'
    input.onchange = () => {
      const file = input.files?.[0]
      if (!file) return
      void uploadMannequinNodeFile(node, file)
    }
    input.click()
  }

  function clearMannequinNodeImage(node: MannequinNodeItem) {
    node.sourceImageUrl = ''
    node.sourceImageRatio = 1
  }

  function onMannequinStageDrop(event: DragEvent, node: MannequinNodeItem) {
    const payload = options.readNodeImageDropPayload(event)
    if (payload) {
      node.sourceImageUrl = payload.url
      if (payload.ratio > 0) node.sourceImageRatio = payload.ratio
      else void syncMannequinNodeImageRatioByUrl(node, payload.url)
      return
    }
    const file = event.dataTransfer?.files?.[0]
    if (!file) return
    void uploadMannequinNodeFile(node, file)
  }

  function importCanvasImageToMannequinNodeAtPoint(clientX: number, clientY: number, item: CanvasImageItem): boolean {
    const target = document.elementFromPoint(clientX, clientY) as HTMLElement | null
    const stageEl = target?.closest('.mannequin-stage[data-mannequin-id]') as HTMLElement | null
    if (!stageEl) return false
    const mannequinId = Number(stageEl.dataset.mannequinId)
    if (!Number.isFinite(mannequinId)) return false
    const node = options.mannequinNodes.value.find(n => n.id === mannequinId)
    if (!node) return false
    node.sourceImageUrl = item.url
    if (item.height_ratio && item.height_ratio > 0) node.sourceImageRatio = item.height_ratio
    else void syncMannequinNodeImageRatioByUrl(node, item.url)
    return true
  }

  return {
    getMannequinStageHeight,
    openMannequinNodeFilePicker,
    clearMannequinNodeImage,
    onMannequinStageDrop,
    importCanvasImageToMannequinNodeAtPoint,
  }
}
