import { computed, ref, type Ref } from 'vue'
import type {
  CanvasImageItem,
  ColorNodeItem,
  EnhanceNodeItem,
  EditNodeItem,
  FusionNodeItem,
  ImageNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PromptNodeItem,
  TryOnNodeItem,
  ViewpointNodeItem,
} from '../types/canvasNodes'

export type NodeDragKind = 'image' | 'prompt' | 'model' | 'viewpoint' | 'mask' | 'color' | 'edit' | 'fusion' | 'enhance' | 'tryon'

export type DraggableNode =
  | ImageNodeItem
  | PromptNodeItem
  | ModelNodeItem
  | ViewpointNodeItem
  | MaskNodeItem
  | ColorNodeItem
  | EditNodeItem
  | FusionNodeItem
  | EnhanceNodeItem
  | TryOnNodeItem

interface UseCanvasSelectionStateOptions {
  messages: Ref<CanvasImageItem[]>
  imageNodes: Ref<ImageNodeItem[]>
  promptNodes: Ref<PromptNodeItem[]>
  modelNodes: Ref<ModelNodeItem[]>
  viewpointNodes: Ref<ViewpointNodeItem[]>
  maskNodes: Ref<MaskNodeItem[]>
  colorNodes: Ref<ColorNodeItem[]>
  editNodes: Ref<EditNodeItem[]>
  fusionNodes: Ref<FusionNodeItem[]>
  enhanceNodes: Ref<EnhanceNodeItem[]>
  tryOnNodes: Ref<TryOnNodeItem[]>
  showStyleStrengthControl: Ref<boolean>
  canvasX: Ref<number>
  canvasY: Ref<number>
  zoomLevel: Ref<number>
  deleteImageNode: (id: number) => void
  deletePromptNode: (id: number) => void
  deleteModelNode: (id: number) => void
  deleteViewpointNode: (id: number) => void
  deleteMaskNode: (id: number) => void
  deleteColorNode: (id: number) => void
  deleteEditNode: (id: number) => void
  deleteFusionNode: (id: number) => void
  deleteEnhanceNode: (id: number) => void
  deleteTryOnNode: (id: number) => void
}

export function useCanvasSelectionState(options: UseCanvasSelectionStateOptions) {
  const selectedIds = ref<number[]>([])
  const selectedNodeKeys = ref<string[]>([])
  const activeImageActionId = ref<number | null>(null)

  const activeImageNodeId = ref<number | null>(null)
  const activePromptNodeId = ref<number | null>(null)
  const activeModelNodeId = ref<number | null>(null)
  const activeViewpointNodeId = ref<number | null>(null)
  const activeMaskNodeId = ref<number | null>(null)
  const activeColorNodeId = ref<number | null>(null)
  const activeEditNodeId = ref<number | null>(null)
  const activeFusionNodeId = ref<number | null>(null)
  const activeEnhanceNodeId = ref<number | null>(null)
  const activeTryOnNodeId = ref<number | null>(null)

  function clearActiveNodes() {
    activeImageNodeId.value = null
    activePromptNodeId.value = null
    activeModelNodeId.value = null
    activeViewpointNodeId.value = null
    activeMaskNodeId.value = null
    activeColorNodeId.value = null
    activeEditNodeId.value = null
    activeFusionNodeId.value = null
    activeEnhanceNodeId.value = null
    activeTryOnNodeId.value = null
  }

  function getNodeSelectionKey(kind: NodeDragKind, id: number): string {
    return `${kind}:${id}`
  }

  function parseNodeSelectionKey(key: string): { kind: NodeDragKind; id: number } | null {
    const [kind, rawId] = key.split(':')
    if (
      kind !== 'image' &&
      kind !== 'prompt' &&
      kind !== 'model' &&
      kind !== 'viewpoint' &&
      kind !== 'mask' &&
      kind !== 'color' &&
      kind !== 'edit' &&
      kind !== 'fusion' &&
      kind !== 'enhance' &&
      kind !== 'tryon'
    ) {
      return null
    }
    const id = Number(rawId)
    if (!Number.isFinite(id)) return null
    return { kind, id }
  }

  function resolveNodeBySelectionKey(key: string): DraggableNode | null {
    const parsed = parseNodeSelectionKey(key)
    if (!parsed) return null
    const { kind, id } = parsed
    if (kind === 'image') return options.imageNodes.value.find(node => node.id === id) ?? null
    if (kind === 'prompt') return options.promptNodes.value.find(node => node.id === id) ?? null
    if (kind === 'model') return options.modelNodes.value.find(node => node.id === id) ?? null
    if (kind === 'viewpoint') return options.viewpointNodes.value.find(node => node.id === id) ?? null
    if (kind === 'mask') return options.maskNodes.value.find(node => node.id === id) ?? null
    if (kind === 'color') return options.colorNodes.value.find(node => node.id === id) ?? null
    if (kind === 'edit') return options.editNodes.value.find(node => node.id === id) ?? null
    if (kind === 'fusion') return options.fusionNodes.value.find(node => node.id === id) ?? null
    if (kind === 'enhance') return options.enhanceNodes.value.find(node => node.id === id) ?? null
    return options.tryOnNodes.value.find(node => node.id === id) ?? null
  }

  function isNodeSelected(kind: NodeDragKind, id: number): boolean {
    const key = getNodeSelectionKey(kind, id)
    if (selectedNodeKeys.value.includes(key)) return true
    if (kind === 'image') return activeImageNodeId.value === id
    if (kind === 'prompt') return activePromptNodeId.value === id
    if (kind === 'model') return activeModelNodeId.value === id
    if (kind === 'viewpoint') return activeViewpointNodeId.value === id
    if (kind === 'mask') return activeMaskNodeId.value === id
    if (kind === 'color') return activeColorNodeId.value === id
    if (kind === 'edit') return activeEditNodeId.value === id
    if (kind === 'fusion') return activeFusionNodeId.value === id
    if (kind === 'enhance') return activeEnhanceNodeId.value === id
    return activeTryOnNodeId.value === id
  }

  function isNodeActive(kind: NodeDragKind, id: number): boolean {
    if (kind === 'image') return activeImageNodeId.value === id
    if (kind === 'prompt') return activePromptNodeId.value === id
    if (kind === 'model') return activeModelNodeId.value === id
    if (kind === 'viewpoint') return activeViewpointNodeId.value === id
    if (kind === 'mask') return activeMaskNodeId.value === id
    if (kind === 'color') return activeColorNodeId.value === id
    if (kind === 'edit') return activeEditNodeId.value === id
    if (kind === 'fusion') return activeFusionNodeId.value === id
    if (kind === 'enhance') return activeEnhanceNodeId.value === id
    return activeTryOnNodeId.value === id
  }

  function clearSelectedNodeKeys() {
    selectedNodeKeys.value = []
  }

  function removeSelectedNodeKey(kind: NodeDragKind, id: number) {
    const key = getNodeSelectionKey(kind, id)
    selectedNodeKeys.value = selectedNodeKeys.value.filter(selectedKey => selectedKey !== key)
  }

  function activateImageNode(id: number) {
    clearActiveNodes()
    activeImageNodeId.value = id
  }

  function activatePromptNode(id: number) {
    clearActiveNodes()
    activePromptNodeId.value = id
  }

  function activateModelNode(id: number) {
    clearActiveNodes()
    activeModelNodeId.value = id
  }

  function activateViewpointNode(id: number) {
    clearActiveNodes()
    activeViewpointNodeId.value = id
  }

  function activateMaskNode(id: number) {
    clearActiveNodes()
    activeMaskNodeId.value = id
  }

  function activateColorNode(id: number) {
    clearActiveNodes()
    activeColorNodeId.value = id
  }

  function activateEditNode(id: number) {
    clearActiveNodes()
    activeEditNodeId.value = id
  }

  function activateFusionNode(id: number) {
    clearActiveNodes()
    activeFusionNodeId.value = id
  }

  function activateEnhanceNode(id: number) {
    clearActiveNodes()
    activeEnhanceNodeId.value = id
  }

  function activateTryOnNode(id: number) {
    clearActiveNodes()
    activeTryOnNodeId.value = id
  }

  function activateNodeByKind(kind: NodeDragKind, id: number) {
    if (kind === 'image') activateImageNode(id)
    else if (kind === 'prompt') activatePromptNode(id)
    else if (kind === 'model') activateModelNode(id)
    else if (kind === 'viewpoint') activateViewpointNode(id)
    else if (kind === 'mask') activateMaskNode(id)
    else if (kind === 'color') activateColorNode(id)
    else if (kind === 'edit') activateEditNode(id)
    else if (kind === 'fusion') activateFusionNode(id)
    else if (kind === 'enhance') activateEnhanceNode(id)
    else if (kind === 'tryon') activateTryOnNode(id)
  }

  function clearCanvasQuickActionState() {
    activeImageActionId.value = null
    options.showStyleStrengthControl.value = false
  }

  function clearSelectedIds() {
    selectedIds.value = []
    clearSelectedNodeKeys()
    clearCanvasQuickActionState()
    clearActiveNodes()
  }

  function deleteNodeByKind(kind: NodeDragKind, id: number) {
    if (kind === 'image') options.deleteImageNode(id)
    else if (kind === 'prompt') options.deletePromptNode(id)
    else if (kind === 'model') options.deleteModelNode(id)
    else if (kind === 'viewpoint') options.deleteViewpointNode(id)
    else if (kind === 'mask') options.deleteMaskNode(id)
    else if (kind === 'color') options.deleteColorNode(id)
    else if (kind === 'edit') options.deleteEditNode(id)
    else if (kind === 'fusion') options.deleteFusionNode(id)
    else if (kind === 'enhance') options.deleteEnhanceNode(id)
    else if (kind === 'tryon') options.deleteTryOnNode(id)
  }

  function deleteSelectedNodes(): boolean {
    if (selectedNodeKeys.value.length === 0) return false
    selectedNodeKeys.value.forEach((selectionKey) => {
      const parsed = parseNodeSelectionKey(selectionKey)
      if (!parsed) return
      deleteNodeByKind(parsed.kind, parsed.id)
    })
    clearSelectedNodeKeys()
    clearActiveNodes()
    return true
  }

  function deleteActiveNode(): boolean {
    if (deleteSelectedNodes()) return true
    if (activeImageNodeId.value !== null) {
      options.deleteImageNode(activeImageNodeId.value)
      return true
    }
    if (activePromptNodeId.value !== null) {
      options.deletePromptNode(activePromptNodeId.value)
      return true
    }
    if (activeModelNodeId.value !== null) {
      options.deleteModelNode(activeModelNodeId.value)
      return true
    }
    if (activeViewpointNodeId.value !== null) {
      options.deleteViewpointNode(activeViewpointNodeId.value)
      return true
    }
    if (activeMaskNodeId.value !== null) {
      options.deleteMaskNode(activeMaskNodeId.value)
      return true
    }
    if (activeColorNodeId.value !== null) {
      options.deleteColorNode(activeColorNodeId.value)
      return true
    }
    if (activeEditNodeId.value !== null) {
      options.deleteEditNode(activeEditNodeId.value)
      return true
    }
    if (activeFusionNodeId.value !== null) {
      options.deleteFusionNode(activeFusionNodeId.value)
      return true
    }
    if (activeEnhanceNodeId.value !== null) {
      options.deleteEnhanceNode(activeEnhanceNodeId.value)
      return true
    }
    if (activeTryOnNodeId.value !== null) {
      options.deleteTryOnNode(activeTryOnNodeId.value)
      return true
    }
    return false
  }

  function deleteSelectedCanvasImages() {
    options.messages.value = options.messages.value.filter(item => !selectedIds.value.includes(item.id))
    selectedIds.value = []
  }

  function removeSelectedId(id: number) {
    selectedIds.value = selectedIds.value.filter(selectedId => selectedId !== id)
    if (activeImageActionId.value === id) {
      activeImageActionId.value = null
    }
    if (activeImageActionId.value === null) {
      options.showStyleStrengthControl.value = false
    }
  }

  function focusSelectedItem(id: number) {
    const target = options.messages.value.find(item => item.id === id)
    if (!target) return
    const zoom = options.zoomLevel.value / 100
    options.canvasX.value = -(target.x * zoom)
    options.canvasY.value = -(target.y * zoom)
  }

  const hasActiveNodeSelection = computed(() =>
      activeImageNodeId.value !== null ||
      activePromptNodeId.value !== null ||
      activeModelNodeId.value !== null ||
      activeViewpointNodeId.value !== null ||
    activeMaskNodeId.value !== null ||
    activeColorNodeId.value !== null ||
    activeEditNodeId.value !== null ||
    activeFusionNodeId.value !== null ||
    activeEnhanceNodeId.value !== null ||
    activeTryOnNodeId.value !== null ||
    selectedNodeKeys.value.length > 0,
  )

  function clearActiveNodeSelection() {
    clearSelectedNodeKeys()
    clearActiveNodes()
  }

  return {
    selectedIds,
    selectedNodeKeys,
    activeImageActionId,
    activeImageNodeId,
    activePromptNodeId,
    activeModelNodeId,
    activeViewpointNodeId,
    activeMaskNodeId,
    activeColorNodeId,
    activeEditNodeId,
    activeFusionNodeId,
    activeEnhanceNodeId,
    activeTryOnNodeId,
    clearActiveNodes,
    getNodeSelectionKey,
    parseNodeSelectionKey,
    resolveNodeBySelectionKey,
    isNodeSelected,
    isNodeActive,
    clearSelectedNodeKeys,
    removeSelectedNodeKey,
    activateImageNode,
    activatePromptNode,
    activateModelNode,
    activateViewpointNode,
    activateMaskNode,
    activateColorNode,
    activateEditNode,
    activateFusionNode,
    activateEnhanceNode,
    activateTryOnNode,
    activateNodeByKind,
    clearCanvasQuickActionState,
    clearSelectedIds,
    deleteSelectedNodes,
    deleteActiveNode,
    deleteSelectedCanvasImages,
    removeSelectedId,
    focusSelectedItem,
    hasActiveNodeSelection,
    clearActiveNodeSelection,
  }
}
