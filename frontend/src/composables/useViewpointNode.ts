import type { Ref } from 'vue'
import type { CanvasImageItem, ImageNodeOutputItem, ViewpointNodeItem } from '../types/canvasNodes'

interface UploadResponse {
  code: number
  msg?: string
  data?: {
    url?: string
  }
}

interface ViewpointNodeMessageApi {
  warning(content: string): unknown
  success(content: string): unknown
  error(content: string): unknown
}

interface DragPayload {
  url: string
  ratio: number
}

interface UseViewpointNodeOptions {
  viewpointNodes: Ref<ViewpointNodeItem[]>
  message: ViewpointNodeMessageApi
  uploadImage: (file: File) => Promise<UploadResponse>
  loadImageRatioFromFile: (file: File) => Promise<number | null>
  loadImageRatioFromUrl: (url: string) => Promise<number | null>
  readNodeImageDropPayload: (event: DragEvent) => DragPayload | null
}

const MAX_VIEWPOINT_SOURCE_IMAGES = 3
const MAX_VIEWPOINT_SELECTIONS = 4

export function useViewpointNode(options: UseViewpointNodeOptions) {
  async function resolveImageRatio(url: string): Promise<number> {
    const ratio = await options.loadImageRatioFromUrl(url)
    return ratio && ratio > 0 ? ratio : 1
  }

  function getViewpointStageHeight(
    node: ViewpointNodeItem,
    sourceImageUrl = node.sourceImages[0]?.url ?? '',
    sourceImageRatio = node.sourceImages[0]?.ratio ?? 1,
  ): number {
    if (!sourceImageUrl) return 176
    const stageWidth = Math.max(120, node.width - 24)
    const ratio = Number.isFinite(sourceImageRatio) && sourceImageRatio > 0
      ? sourceImageRatio
      : 1
    const dynamicHeight = Math.round(stageWidth * ratio)
    return Math.max(120, Math.min(420, dynamicHeight))
  }

  function setNodeImages(node: ViewpointNodeItem, images: ImageNodeOutputItem[]) {
    node.sourceImages = images.slice(0, MAX_VIEWPOINT_SOURCE_IMAGES).map((image) => ({
      url: image.url,
      ratio: image.ratio > 0 ? image.ratio : 1,
    }))
  }

  async function appendSourceImage(node: ViewpointNodeItem, image: { url: string; ratio?: number | null }, reset = false) {
    if (!image.url) return
    const ratio = image.ratio && image.ratio > 0 ? image.ratio : await resolveImageRatio(image.url)
    const nextImages = reset ? [] : [...node.sourceImages]
    nextImages.push({
      url: image.url,
      ratio,
    })
    setNodeImages(node, nextImages)
  }

  async function uploadViewpointFiles(node: ViewpointNodeItem, fileList: FileList | File[]) {
    const files = Array.from(fileList).filter(file => file.type.startsWith('image/'))
    if (files.length === 0) {
      options.message.warning('请上传图片文件')
      return
    }

    const remainingSlots = Math.max(0, MAX_VIEWPOINT_SOURCE_IMAGES - node.sourceImages.length)
    if (remainingSlots <= 0) {
      options.message.warning('最多添加 3 张图片')
      return
    }

    const acceptedFiles = files.slice(0, remainingSlots)
    if (acceptedFiles.length < files.length) {
      options.message.warning('最多添加 3 张图片')
    }

    try {
      const uploadedImages: ImageNodeOutputItem[] = []
      for (const file of acceptedFiles) {
        const ratioFromFile = await options.loadImageRatioFromFile(file)
        const res = await options.uploadImage(file)
        if (res.code !== 0 || !res.data?.url) {
          options.message.error(res.msg || '上传失败')
          return
        }
        uploadedImages.push({
          url: res.data.url,
          ratio: ratioFromFile && ratioFromFile > 0 ? ratioFromFile : 1,
        })
      }

      setNodeImages(node, [...node.sourceImages, ...uploadedImages])
      options.message.success('Viewpoint 节点图片已更新')
    } catch (error: unknown) {
      const maybeError = error as { msg?: string; message?: string } | null
      options.message.error(maybeError?.msg || maybeError?.message || '上传失败')
    }
  }

  function openViewpointNodeFilePicker(node: ViewpointNodeItem) {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = 'image/*'
    input.multiple = true
    input.onchange = () => {
      const files = input.files
      if (!files || files.length === 0) return
      void uploadViewpointFiles(node, files)
    }
    input.click()
  }

  function clearViewpointNodeImages(node: ViewpointNodeItem) {
    node.sourceImages = []
  }

  function removeViewpointNodeImage(node: ViewpointNodeItem, index: number) {
    if (index < 0 || index >= node.sourceImages.length) return
    node.sourceImages.splice(index, 1)
  }

  function onViewpointStageDrop(event: DragEvent, node: ViewpointNodeItem) {
    const payload = options.readNodeImageDropPayload(event)
    if (payload) {
      void appendSourceImage(node, { url: payload.url, ratio: payload.ratio })
      return
    }

    const files = event.dataTransfer?.files
    if (!files || files.length === 0) return
    void uploadViewpointFiles(node, files)
  }

  function importCanvasImageToViewpointNodeAtPoint(clientX: number, clientY: number, item: CanvasImageItem): boolean {
    const target = document.elementFromPoint(clientX, clientY) as HTMLElement | null
    const stageEl = target?.closest('.viewpoint-stage[data-viewpoint-id]') as HTMLElement | null
    if (!stageEl) return false
    const viewpointId = Number(stageEl.dataset.viewpointId)
    if (!Number.isFinite(viewpointId)) return false
    const node = options.viewpointNodes.value.find(n => n.id === viewpointId)
    if (!node) return false
    if (node.sourceImages.length >= MAX_VIEWPOINT_SOURCE_IMAGES) {
      options.message.warning('最多添加 3 张图片')
      return false
    }
    void appendSourceImage(node, { url: item.url, ratio: item.height_ratio }, false)
    return true
  }

  function toggleViewpointSelection(node: ViewpointNodeItem, key: string) {
    const currentIndex = node.selectedViewpoints.indexOf(key)
    if (currentIndex >= 0) {
      node.selectedViewpoints.splice(currentIndex, 1)
      return
    }
    if (node.selectedViewpoints.length >= MAX_VIEWPOINT_SELECTIONS) {
      options.message.warning('最多选择 4 个视角')
      return
    }
    node.selectedViewpoints.push(key)
  }

  return {
    getViewpointStageHeight,
    openViewpointNodeFilePicker,
    clearViewpointNodeImages,
    removeViewpointNodeImage,
    onViewpointStageDrop,
    importCanvasImageToViewpointNodeAtPoint,
    toggleViewpointSelection,
  }
}
