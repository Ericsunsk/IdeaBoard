import { computed, ref, watch } from 'vue'
import { useFileDialog } from '@vueuse/core'
import { IMAGE_UPLOAD_ACCEPT } from '../../../../shared/constants/media'
import { clampImageNodeOutputCount } from '../../../canvas-board/domain'
import {
  clampMaskLayerOpacity,
  MASK_EDITOR_DEFAULT_LAYER_OPACITY,
  MASK_EDITOR_REFERENCE_IMAGE_LIMIT,
} from '../config/maskEditorConstants'
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

export function useMaskEditorChromeState(options: UseMaskEditorChromeStateOptions) {
  const maskEditorLayerOpacity = ref(MASK_EDITOR_DEFAULT_LAYER_OPACITY)
  const maskEditorReferenceImages = ref<MaskEditorReferenceImage[]>([])
  const referenceImageDialog = useFileDialog({
    accept: IMAGE_UPLOAD_ACCEPT,
    multiple: false,
    reset: true,
  })

  const activeMaskEditorOutputCount = computed(() => {
    const nodeId = options.imageEditorNodeId.value
    if (!nodeId) return 1
    const snapshot = options.getImageNodeSnapshot(nodeId)
    return snapshot ? clampImageNodeOutputCount(snapshot.outputCount) : 1
  })

  function setMaskEditorLayerOpacity(value: number) {
    maskEditorLayerOpacity.value = clampMaskLayerOpacity(value)
  }

  function onMaskEditorReferenceImageOpen() {
    if (maskEditorReferenceImages.value.length >= MASK_EDITOR_REFERENCE_IMAGE_LIMIT) {
      options.onWarning(`局部重绘最多添加 ${MASK_EDITOR_REFERENCE_IMAGE_LIMIT} 张参考图`)
      return
    }
    referenceImageDialog.open()
  }

  referenceImageDialog.onChange((files) => {
    const file = files?.[0]
    if (!file) return
    void addMaskEditorReferenceImage(file)
  })

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
      maskEditorLayerOpacity.value = MASK_EDITOR_DEFAULT_LAYER_OPACITY
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
