import { computed, ref, watch } from 'vue'
import { IMAGE_UPLOAD_ACCEPT } from '../../../../shared/constants/media'
import { clampImageNodeOutputCount, getCanvasNodeImageInputLimit } from '../../../canvas-board/domain'
import type { MaskEditorReferenceImage } from '../types'

type ValueRef<T> = {
  readonly value: T
}

type UseMaskEditorChromeStateOptions = {
  imageEditorNodeId: ValueRef<string | null>
  getImageNodeSnapshot: (nodeId: string) => { outputCount?: number } | null
  uploadImageToBoard: (file: File) => Promise<{ url: string; ratio: number }>
  onWarning: (message: string) => void
  onReferenceImagePreview?: (image: { url: string; ratio: number }) => void
}

const MASK_EDITOR_REFERENCE_IMAGE_LIMIT = getCanvasNodeImageInputLimit('prompt')

export function useMaskEditorChromeState(options: UseMaskEditorChromeStateOptions) {
  const maskEditorLayerOpacity = ref(72)
  const maskEditorReferenceImages = ref<MaskEditorReferenceImage[]>([])

  const activeMaskEditorOutputCount = computed(() => {
    const nodeId = options.imageEditorNodeId.value
    if (!nodeId) return 1
    const snapshot = options.getImageNodeSnapshot(nodeId)
    return snapshot ? clampImageNodeOutputCount(snapshot.outputCount) : 1
  })

  function setMaskEditorLayerOpacity(value: number) {
    maskEditorLayerOpacity.value = Math.max(15, Math.min(100, Number(value) || 72))
  }

  function onMaskEditorReferenceImageOpen() {
    if (maskEditorReferenceImages.value.length >= MASK_EDITOR_REFERENCE_IMAGE_LIMIT) {
      options.onWarning(`局部重绘最多添加 ${MASK_EDITOR_REFERENCE_IMAGE_LIMIT} 张参考图`)
      return
    }
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = IMAGE_UPLOAD_ACCEPT
    input.onchange = () => {
      const file = input.files?.[0]
      if (!file) return
      void addMaskEditorReferenceImage(file)
    }
    input.click()
  }

  async function addMaskEditorReferenceImage(file: File) {
    if (maskEditorReferenceImages.value.length >= MASK_EDITOR_REFERENCE_IMAGE_LIMIT) {
      options.onWarning(`局部重绘最多添加 ${MASK_EDITOR_REFERENCE_IMAGE_LIMIT} 张参考图`)
      return
    }
    try {
      const uploaded = await options.uploadImageToBoard(file)
      maskEditorReferenceImages.value = [
        ...maskEditorReferenceImages.value,
        {
          id: `mask-ref:${Date.now()}:${Math.random().toString(16).slice(2, 8)}`,
          url: uploaded.url,
          ratio: uploaded.ratio,
        },
      ].slice(0, MASK_EDITOR_REFERENCE_IMAGE_LIMIT)
    } catch (error) {
      options.onWarning(error instanceof Error ? error.message : '上传失败')
    }
  }

  function onMaskEditorReferenceImageRemove(imageId: string) {
    maskEditorReferenceImages.value = maskEditorReferenceImages.value.filter(
      (image) => image.id !== imageId,
    )
  }

  function onMaskEditorReferenceImagePreview(image: { url: string; ratio?: number }) {
    options.onReferenceImagePreview?.({
      url: image.url,
      ratio: Number(image.ratio) || 1,
    })
  }

  watch(
    () => options.imageEditorNodeId.value,
    () => {
      maskEditorLayerOpacity.value = 72
      maskEditorReferenceImages.value = []
    },
  )

  return {
    maskEditorLayerOpacity,
    maskEditorReferenceImages,
    activeMaskEditorOutputCount,
    setMaskEditorLayerOpacity,
    onMaskEditorReferenceImageOpen,
    onMaskEditorReferenceImageRemove,
    onMaskEditorReferenceImagePreview,
  }
}
