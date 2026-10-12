import { computed } from 'vue'
import {
  resolveImageNodeDisplayRatio,
  resolveImageNodeStageHeight,
  type FusionNodeItem,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../../../../board/ports/canvas'
import type { NodeUiState, PromptNodeUiState } from '../../../core/nodes/canvasNodeUi.ts'
import type { FlowNodeData } from '../../../core/nodes/flowTypes.ts'
import { resolveImageNodeDisplayPayload } from './flowImageNodeSelectors.ts'

interface UseFlowCanvasNodeAdapterOptions {
  data: () => FlowNodeData
  uiState: () => NodeUiState | PromptNodeUiState | undefined
  imageDirectGenerating: () => boolean
  outpaintModeActive: () => boolean
  isRetouchActive: () => boolean
}

function resolveSnapshotNode<T>(data: FlowNodeData) {
  return data.snapshotNode && typeof data.snapshotNode === 'object'
    ? ({ ...(data.snapshotNode as T) } as T)
    : null
}

export function useFlowCanvasNodeAdapter({
  data,
  uiState,
  imageDirectGenerating,
  outpaintModeActive,
  isRetouchActive,
}: UseFlowCanvasNodeAdapterOptions) {
  const isPromptNode = computed(() => data().kind === 'prompt')
  const isImageNode = computed(() => data().kind === 'image')
  const isFusionNode = computed(() => data().kind === 'fusion')
  const shouldDisableHandleRailInteraction = computed(
    () => isImageNode.value && (outpaintModeActive() || isRetouchActive()),
  )
  const promptUiState = computed<PromptNodeUiState | undefined>(
    () => uiState() as PromptNodeUiState | undefined,
  )

  const promptSnapshot = computed<PromptNodeItem | null>(() => {
    if (!isPromptNode.value) return null
    return resolveSnapshotNode<PromptNodeItem>(data())
  })

  const imageSnapshot = computed<ImageNodeItem | null>(() => {
    if (!isImageNode.value) return null
    return resolveSnapshotNode<ImageNodeItem>(data())
  })

  const fusionSnapshot = computed<FusionNodeItem | null>(() => {
    if (!isFusionNode.value) return null
    return resolveSnapshotNode<FusionNodeItem>(data())
  })

  const displayImage = computed<{ url: string; assetId?: string | undefined } | null>(() => {
    if (!imageSnapshot.value) return null
    const pendingPreview = data().libraryImportPreviewUrl?.trim()
    if (imageSnapshot.value.libraryImport && pendingPreview) return { url: pendingPreview }
    const localUploadPreview = data().localUpload?.previewUrl.trim()
    if (localUploadPreview) return { url: localUploadPreview }
    const payload = resolveImageNodeDisplayPayload(imageSnapshot.value)
    return payload
      ? { url: payload.url, ...(payload.assetId ? { assetId: payload.assetId } : {}) }
      : null
  })
  const libraryImport = computed(() => imageSnapshot.value?.libraryImport)
  const localUpload = computed(() => data().localUpload)

  const isImageGenerating = computed(() => imageDirectGenerating())
  const isNodeRunning = computed(() => isImageGenerating.value || !!uiState()?.isRunning)
  const isImageInputLocked = computed(
    () =>
      !!libraryImport.value ||
      !!localUpload.value ||
      (isImageGenerating.value && !displayImage.value),
  )
  const imageEmptyIconVariant = computed<'image' | 'fusion'>(() => {
    const imageUiState = uiState() as NodeUiState<'image'> | undefined
    return imageUiState?.connection?.sourceKind?.image === 'fusion' ? 'fusion' : 'image'
  })

  const stageHeight = computed(() => {
    if (!imageSnapshot.value) return 280
    const hasImageFrame = !!displayImage.value || isImageGenerating.value
    const ratio = resolveImageNodeDisplayRatio(imageSnapshot.value)
    return resolveImageNodeStageHeight(imageSnapshot.value.width, ratio, hasImageFrame)
  })

  return {
    shouldDisableHandleRailInteraction,
    displayImage,
    fusionSnapshot,
    imageEmptyIconVariant,
    isImageGenerating,
    isImageInputLocked,
    libraryImport,
    localUpload,
    imageSnapshot,
    isFusionNode,
    isImageNode,
    isPromptNode,
    isNodeRunning,
    promptSnapshot,
    promptUiState,
    stageHeight,
  }
}
