import { nextTick, type Ref } from 'vue'
import { normalizeHexColor } from '../components/nodes/colorPopoverUtils'
import { normalizeViewpointSelectionByCategory } from '../components/nodes/nodeOptionConfig'
import { clampMaskRefineAmount } from './useMaskNode'
import {
  DEFAULT_COLOR_STRENGTH,
  ENHANCE_EXTEND_PERCENT_DEFAULT,
  clampEnhanceExtendPercent,
  clampModelPoseStrength,
  clampModelReferenceStrength,
  clampWearOpacity,
  clampWearRotation,
  clampWearScale,
  normalizeColorNodeColors,
  normalizeWearTileMode,
  resolveWorkflowNodeOutputStatus,
} from '../types/canvasNodes'
import type {
  CanvasBoardSnapshot,
  CanvasConnection,
  CanvasImageItem,
  ColorNodeItem,
  EditNodeItem,
  EnhanceNodeItem,
  FusionNodeItem,
  ImageNodeItem,
  MaskNodeItem,
  ModelAngleSpec,
  ModelNodeItem,
  PromptNodeItem,
  ViewpointNodeItem,
  WearNodeItem,
} from '../types/canvasNodes'

interface UseWorkspaceBoardSnapshotOptions {
  messages: Ref<CanvasImageItem[]>
  imageNodes: Ref<ImageNodeItem[]>
  promptNodes: Ref<PromptNodeItem[]>
  modelNodes: Ref<ModelNodeItem[]>
  viewpointNodes: Ref<ViewpointNodeItem[]>
  maskNodes: Ref<MaskNodeItem[]>
  colorNodes: Ref<ColorNodeItem[]>
  enhanceNodes: Ref<EnhanceNodeItem[]>
  editNodes: Ref<EditNodeItem[]>
  fusionNodes: Ref<FusionNodeItem[]>
  wearNodes: Ref<WearNodeItem[]>
  connections: Ref<CanvasConnection[]>
  runningExecutionNodeKeys: Ref<Set<string>>
  executionNodeLastRunFailedMap: Ref<Record<string, boolean>>
  processedMaskPreviewUrls: Ref<Record<number, string>>
  resetEditorState: () => void
  clearAllNodeConnectionErrors: () => void
  resetEditorUiState: () => void
  clearSelectedConnection: () => void
  clearConnectionDraft: () => void
  setViewport: (viewport: CanvasBoardSnapshot['viewport']) => void
  createViewportSnapshot: () => CanvasBoardSnapshot['viewport']
  renderMaskNodeMask: (node: MaskNodeItem) => void
  refreshProcessedMaskPreview: (node: MaskNodeItem) => Promise<void>
  clearProcessedMaskPreview: (id: number) => void
  scheduleRenderedConnectionsRefresh: () => void
  resetHistoryBaseline: (snapshot?: CanvasBoardSnapshot | null) => void
  setSuspendWorkflowCacheReactions: (next: boolean) => void
}

function cloneNodeOutputImages(outputs: { url: string; ratio: number }[]) {
  return outputs.map(output => ({ ...output }))
}

function cloneModelAngleSpec(angleSpec: ModelAngleSpec) {
  return { ...angleSpec }
}

export function useWorkspaceBoardSnapshot(options: UseWorkspaceBoardSnapshotOptions) {
  const {
    messages,
    imageNodes,
    promptNodes,
    modelNodes,
    viewpointNodes,
    maskNodes,
    colorNodes,
    enhanceNodes,
    editNodes,
    fusionNodes,
    wearNodes,
    connections,
    runningExecutionNodeKeys,
    executionNodeLastRunFailedMap,
    processedMaskPreviewUrls,
    resetEditorState,
    clearAllNodeConnectionErrors,
    resetEditorUiState,
    clearSelectedConnection,
    clearConnectionDraft,
    setViewport,
    createViewportSnapshot,
    renderMaskNodeMask,
    refreshProcessedMaskPreview,
    clearProcessedMaskPreview,
    scheduleRenderedConnectionsRefresh,
    resetHistoryBaseline,
    setSuspendWorkflowCacheReactions,
  } = options

  function toNormalizedColorNode(node: ColorNodeItem): ColorNodeItem {
    const colors = normalizeColorNodeColors(node.colors.map(color => normalizeHexColor(color)))
    return {
      ...node,
      outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
      colors,
      strength: Math.max(0, Math.min(100, Math.round(node.strength ?? DEFAULT_COLOR_STRENGTH))),
      outputImages: cloneNodeOutputImages(node.outputImages),
    }
  }

  function createBoardSnapshotBase(): CanvasBoardSnapshot {
    return {
      messages: messages.value.map(item => ({ ...item })),
      imageNodes: imageNodes.value.map(node => ({
        ...node,
        outputImages: cloneNodeOutputImages(node.outputImages),
      })),
      promptNodes: promptNodes.value.map(node => ({ ...node })),
      modelNodes: modelNodes.value.map(node => ({
        ...node,
        angleSpec: cloneModelAngleSpec(node.angleSpec),
        outputImages: cloneNodeOutputImages(node.outputImages),
      })),
      viewpointNodes: viewpointNodes.value.map(node => ({
        ...node,
        outputImages: cloneNodeOutputImages(node.outputImages),
      })),
      maskNodes: maskNodes.value.map(node => ({ ...node })),
      colorNodes: colorNodes.value.map(toNormalizedColorNode),
      enhanceNodes: enhanceNodes.value.map(node => ({
        ...node,
        outputImages: cloneNodeOutputImages(node.outputImages),
      })),
      editNodes: editNodes.value.map(node => ({
        ...node,
        outputImages: cloneNodeOutputImages(node.outputImages),
      })),
      fusionNodes: fusionNodes.value.map(node => ({
        ...node,
        outputImages: cloneNodeOutputImages(node.outputImages),
      })),
      wearNodes: wearNodes.value.map(node => ({
        ...node,
        outputImages: cloneNodeOutputImages(node.outputImages),
      })),
      connections: connections.value.map(connection => ({ ...connection })),
      viewport: createViewportSnapshot(),
    }
  }

  async function applyBoardSnapshot(state: CanvasBoardSnapshot | null) {
    setSuspendWorkflowCacheReactions(true)
    try {
      if (!state) {
        resetEditorState()
        await nextTick()
        resetHistoryBaseline(createBoardSnapshotBase())
        return
      }

      messages.value = state.messages.map(item => ({ ...item }))
      resetEditorUiState()
      clearAllNodeConnectionErrors()
      runningExecutionNodeKeys.value = new Set()
      executionNodeLastRunFailedMap.value = {}

      imageNodes.value = state.imageNodes.map(node => ({
        ...node,
        outputImages: cloneNodeOutputImages(node.outputImages),
      }))

      promptNodes.value = state.promptNodes.map(node => ({ ...node }))

      modelNodes.value = state.modelNodes.map(node => ({
        ...node,
        modelStrength: clampModelReferenceStrength(node.modelStrength),
        poseStrength: clampModelPoseStrength(node.poseStrength),
        outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
        angleSpec: cloneModelAngleSpec(node.angleSpec),
        outputImages: cloneNodeOutputImages(node.outputImages),
      }))
      viewpointNodes.value = state.viewpointNodes.map(node => ({
        ...node,
        selectedViewpoint: normalizeViewpointSelectionByCategory(node.activeCategory, node.selectedViewpoint),
        outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
        outputImages: cloneNodeOutputImages(node.outputImages),
      }))
      maskNodes.value = state.maskNodes.map(node => ({
        ...node,
        refineAmount: clampMaskRefineAmount(node.refineAmount ?? 0),
        invertMask: !!node.invertMask,
      }))
      enhanceNodes.value = state.enhanceNodes.map(node => ({
        ...node,
        outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
        outputQuality: Math.max(30, Math.min(100, Math.round(node.outputQuality ?? 95))),
        extendTop: clampEnhanceExtendPercent(node.extendTop ?? ENHANCE_EXTEND_PERCENT_DEFAULT),
        extendRight: clampEnhanceExtendPercent(node.extendRight ?? ENHANCE_EXTEND_PERCENT_DEFAULT),
        extendBottom: clampEnhanceExtendPercent(node.extendBottom ?? ENHANCE_EXTEND_PERCENT_DEFAULT),
        extendLeft: clampEnhanceExtendPercent(node.extendLeft ?? ENHANCE_EXTEND_PERCENT_DEFAULT),
        outputImages: cloneNodeOutputImages(node.outputImages),
      }))
      editNodes.value = state.editNodes.map(node => ({
        ...node,
        outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
        outputImages: cloneNodeOutputImages(node.outputImages),
      }))
      fusionNodes.value = state.fusionNodes.map(node => ({
        ...node,
        blendRatio: Math.max(0, Math.min(100, Math.round(node.blendRatio ?? 50))),
        outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
        outputImages: cloneNodeOutputImages(node.outputImages),
      }))
      wearNodes.value = state.wearNodes.map(node => ({
        ...node,
        tileMode: normalizeWearTileMode(node.tileMode),
        outputStatus: resolveWorkflowNodeOutputStatus(node.outputStatus, node.outputImages),
        rotation: clampWearRotation(node.rotation),
        scale: clampWearScale(node.scale),
        opacity: clampWearOpacity(node.opacity),
        outputImages: cloneNodeOutputImages(node.outputImages),
      }))
      colorNodes.value = state.colorNodes.map(toNormalizedColorNode)
      connections.value = state.connections.map(connection => ({ ...connection }))
      clearSelectedConnection()
      clearConnectionDraft()
      setViewport(state.viewport)
      await nextTick()
      const nextMaskNodeIds = new Set(maskNodes.value.map(node => node.id))
      Object.keys(processedMaskPreviewUrls.value).forEach((rawId) => {
        const id = Number(rawId)
        if (!nextMaskNodeIds.has(id)) {
          clearProcessedMaskPreview(id)
        }
      })
      maskNodes.value.forEach(node => renderMaskNodeMask(node))
      maskNodes.value.forEach((node) => {
        void refreshProcessedMaskPreview(node)
      })
      scheduleRenderedConnectionsRefresh()
      resetHistoryBaseline(createBoardSnapshotBase())
    } finally {
      setSuspendWorkflowCacheReactions(false)
    }
  }

  return {
    createBoardSnapshotBase,
    applyBoardSnapshot,
  }
}
