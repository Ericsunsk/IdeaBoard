import { computed } from 'vue'
import type { FusionNodeItem, ImageNodeItem, PromptNodeItem } from '../../canvas-board/domain'
import type { NodeUiState, PromptNodeUiState } from '../types/canvasNodeUi'
import type { FlowNodeData } from '../types/flowTypes'
import { resolveImageNodeStageHeight } from '../../canvas-board/domain'
import {
  resolveImageNodeDisplayPayload,
  resolveImageNodeDisplayRatio,
} from '../utils/flowImageNodeSelectors'

interface UseFlowCanvasNodeAdapterOptions {
  data: () => FlowNodeData
  uiState: () => NodeUiState | PromptNodeUiState | undefined
  imageDirectGenerating: () => boolean
  expandModeActive: () => boolean
  isImageEditorActive: () => boolean
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
  expandModeActive,
  isImageEditorActive,
}: UseFlowCanvasNodeAdapterOptions) {
  const isPromptNode = computed(() => data().kind === 'prompt')
  const isImageNode = computed(() => data().kind === 'image')
  const isFusionNode = computed(() => data().kind === 'fusion')
  const disableHandleRailInteraction = computed(
    () => isImageNode.value && (expandModeActive() || isImageEditorActive()),
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

  const displayImage = computed<{ url: string } | null>(() => {
    if (!imageSnapshot.value) return null
    const payload = resolveImageNodeDisplayPayload(imageSnapshot.value)
    return payload ? { url: payload.url } : null
  })

  const imageGenerating = computed(
    () => imageDirectGenerating() || imageSnapshot.value?.outputStatus === 'running',
  )
  const nodeRunning = computed(() => imageGenerating.value || !!uiState()?.isRunning)
  const imageInputLocked = computed(() => imageGenerating.value && !displayImage.value)
  const imageEmptyIconVariant = computed<'image' | 'fusion'>(() => {
    const imageUiState = uiState() as NodeUiState<'image'> | undefined
    return imageUiState?.connection?.sourceKind?.image === 'fusion' ? 'fusion' : 'image'
  })

  const stageHeight = computed(() => {
    if (!imageSnapshot.value) return 280
    const hasRunningOutput = imageSnapshot.value.outputStatus === 'running'
    const hasImageFrame = !!displayImage.value || hasRunningOutput
    const ratio = resolveImageNodeDisplayRatio(imageSnapshot.value)
    return resolveImageNodeStageHeight(imageSnapshot.value.width, ratio, hasImageFrame)
  })

  return {
    disableHandleRailInteraction,
    displayImage,
    fusionSnapshot,
    imageEmptyIconVariant,
    imageGenerating,
    imageInputLocked,
    imageSnapshot,
    isFusionNode,
    isImageNode,
    isPromptNode,
    nodeRunning,
    promptSnapshot,
    promptUiState,
    stageHeight,
  }
}
