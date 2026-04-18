import { onMounted, onUnmounted, type Ref } from 'vue'
import type {
  CanvasImageItem,
  CanvasNodeItem,
  ColorNodeItem,
  EditNodeItem,
  EnhanceNodeItem,
  FaceSwapNodeItem,
  FusionNodeItem,
  GenerateNodeItem,
  ImageNodeItem,
  MannequinNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PromptNodeItem,
  TryOnNodeItem,
  ViewpointNodeItem,
} from '../types/canvasNodes'

type NodeDragKind = 'image' | 'prompt' | 'model' | 'mannequin' | 'viewpoint' | 'mask' | 'color' | 'edit' | 'fusion' | 'enhance' | 'tryon' | 'faceswap' | 'generate'
type DraggableNode =
  | ImageNodeItem
  | PromptNodeItem
  | ModelNodeItem
  | MannequinNodeItem
  | ViewpointNodeItem
  | MaskNodeItem
  | ColorNodeItem
  | EditNodeItem
  | FusionNodeItem
  | EnhanceNodeItem
  | TryOnNodeItem
  | FaceSwapNodeItem
  | GenerateNodeItem
type PendingInteraction = 'pan' | 'select' | 'item-drag' | 'node-drag' | 'resize'

const POINTER_INTERACTION_THRESHOLD = 4

interface MessageApiLike {
  success(content: string): unknown
}

interface UseCanvasPointerInteractionsOptions {
  messages: Ref<CanvasImageItem[]>
  imageNodes: Ref<ImageNodeItem[]>
  promptNodes: Ref<PromptNodeItem[]>
  modelNodes: Ref<ModelNodeItem[]>
  mannequinNodes: Ref<MannequinNodeItem[]>
  viewpointNodes: Ref<ViewpointNodeItem[]>
  maskNodes: Ref<MaskNodeItem[]>
  colorNodes: Ref<ColorNodeItem[]>
  editNodes: Ref<EditNodeItem[]>
  fusionNodes: Ref<FusionNodeItem[]>
  enhanceNodes: Ref<EnhanceNodeItem[]>
  tryOnNodes: Ref<TryOnNodeItem[]>
  faceSwapNodes: Ref<FaceSwapNodeItem[]>
  generateNodes: Ref<GenerateNodeItem[]>
  activeTool: Ref<string>
  showStyleStrengthControl: Ref<boolean>
  canvasX: Ref<number>
  canvasY: Ref<number>
  zoomLevel: Ref<number>
  canvasLayoutRef: Ref<HTMLElement | null>
  selectedIds: Ref<number[]>
  selectedNodeKeys: Ref<string[]>
  activeImageActionId: Ref<number | null>
  activeImageNodeId: Ref<number | null>
  activePromptNodeId: Ref<number | null>
  activeModelNodeId: Ref<number | null>
  activeMannequinNodeId: Ref<number | null>
  activeViewpointNodeId: Ref<number | null>
  activeMaskNodeId: Ref<number | null>
  activeColorNodeId: Ref<number | null>
  activeEditNodeId: Ref<number | null>
  activeFusionNodeId: Ref<number | null>
  activeEnhanceNodeId: Ref<number | null>
  activeTryOnNodeId: Ref<number | null>
  activeFaceSwapNodeId: Ref<number | null>
  activeGenerateNodeId: Ref<number | null>
  isPanning: Ref<boolean>
  isSelecting: Ref<boolean>
  isDraggingItem: Ref<boolean>
  draggingNodeKeys: Ref<string[]>
  isResizingItem: Ref<boolean>
  isSpacePressed: Ref<boolean>
  selectionRect: Ref<{ x1: number; y1: number; x2: number; y2: number }>
  nodeContextMenu: Ref<{ visible: boolean; x: number; y: number; clientX: number; clientY: number }>
  isBoardMenuOpen: () => boolean
  setBoardMenuOpen: (visible: boolean) => void
  isBoardRenaming: () => boolean
  confirmBoardRename: () => void
  clearSelectedConnection: () => void
  clearSelectedNodeKeys: () => void
  removeSelectedNodeKey: (kind: NodeDragKind, id: number) => void
  clearActiveNodes: () => void
  activateNodeByKind: (kind: NodeDragKind, id: number) => void
  isNodeActive: (kind: NodeDragKind, id: number) => boolean
  resolveNodeBySelectionKey: (key: string) => DraggableNode | null
  getNodeSelectionKey: (kind: NodeDragKind, id: number) => string
  beginHistoryBatch: () => void
  endHistoryBatch: (commit?: boolean) => void
  importCanvasImageToImageNodeAtPoint: (clientX: number, clientY: number, imageItem: CanvasImageItem) => boolean
  importCanvasImageToMaskNodeAtPoint: (clientX: number, clientY: number, imageItem: CanvasImageItem) => boolean
  importCanvasImageToModelNodeAtPoint: (clientX: number, clientY: number, imageItem: CanvasImageItem) => boolean
  importCanvasImageToMannequinNodeAtPoint: (clientX: number, clientY: number, imageItem: CanvasImageItem) => boolean
  importCanvasImageToViewpointNodeAtPoint: (clientX: number, clientY: number, imageItem: CanvasImageItem) => boolean
  message: MessageApiLike
}

function createCanvasEntityId(): number {
  return Date.now() + Math.random()
}

export function useCanvasPointerInteractions(options: UseCanvasPointerInteractionsOptions) {
  let startX = 0
  let startY = 0
  let initialX = 0
  let initialY = 0
  let initialW = 0
  let initialH = 0
  let resizeDir = ''
  let draggedCanvasItem: CanvasImageItem | null = null
  let draggedSelectionOffsets = new Map<number, { x: number; y: number }>()
  let draggedNodeOffsets = new Map<string, { node: DraggableNode; x: number; y: number }>()
  let draggedNodePrimaryKey: string | null = null
  let pendingInteraction: PendingInteraction | null = null
  let pendingDuplicateCanvasSourceIds: number[] = []
  let pendingDuplicateNodeKeys: string[] = []
  let marqueeBaseSelectionIds: number[] = []
  let marqueeBaseNodeSelectionKeys: string[] = []
  let nodeDragClientX = 0
  let nodeDragClientY = 0
  let nodeDragRAF: number | null = null
  let historyBatchActive = false

  function closeNodeContextMenu() {
    if (!options.nodeContextMenu.value.visible) return
    options.nodeContextMenu.value.visible = false
  }

  function clearCanvasQuickActionState() {
    options.activeImageActionId.value = null
    options.showStyleStrengthControl.value = false
  }

  function beginHistoryBatch() {
    if (historyBatchActive) return
    historyBatchActive = true
    options.beginHistoryBatch()
  }

  function endHistoryBatch(commit = true) {
    if (!historyBatchActive) return
    historyBatchActive = false
    options.endHistoryBatch(commit)
  }

  function clearPendingInteraction() {
    pendingInteraction = null
    pendingDuplicateCanvasSourceIds = []
    pendingDuplicateNodeKeys = []
  }

  function hasExceededPointerThreshold(clientX: number, clientY: number): boolean {
    return Math.hypot(clientX - startX, clientY - startY) >= POINTER_INTERACTION_THRESHOLD
  }

  function cloneCanvasItem(item: CanvasImageItem): CanvasImageItem {
    return {
      ...item,
      id: createCanvasEntityId(),
    }
  }

  function cloneCanvasNode(node: CanvasNodeItem): CanvasNodeItem {
    if (node.type === 'prompt') {
      return { ...node, id: createCanvasEntityId() }
    }
    if (node.type === 'model') {
      return {
        ...node,
        id: createCanvasEntityId(),
        selectedAngles: [...node.selectedAngles],
        outputImages: node.outputImages.map(output => ({ ...output })),
      }
    }
    if (node.type === 'mannequin') {
      return {
        ...node,
        id: createCanvasEntityId(),
        outputImages: node.outputImages.map(output => ({ ...output })),
      }
    }
    if (node.type === 'viewpoint') {
      return {
        ...node,
        id: createCanvasEntityId(),
        sourceImages: node.sourceImages.map(image => ({ ...image })),
        selectedViewpoints: [...node.selectedViewpoints],
        outputImages: node.outputImages.map(output => ({ ...output })),
      }
    }
    if (node.type === 'mask') {
      return {
        ...node,
        id: createCanvasEntityId(),
      }
    }
    if (node.type === 'image-node') {
      return {
        ...node,
        id: createCanvasEntityId(),
        outputImages: node.outputImages.map(output => ({ ...output })),
      }
    }
    if (node.type === 'color') {
      return {
        ...node,
        id: createCanvasEntityId(),
        outputImages: node.outputImages.map(output => ({ ...output })),
      }
    }
    if (
      node.type === 'edit' ||
      node.type === 'fusion' ||
      node.type === 'enhance' ||
      node.type === 'tryon' ||
      node.type === 'faceswap' ||
      node.type === 'generate'
    ) {
      return {
        ...node,
        id: createCanvasEntityId(),
        outputImages: node.outputImages.map(output => ({ ...output })),
      }
    }
    throw new Error('Unsupported canvas node type')
  }

  function applyDuplicatedCanvasSelection() {
    if (!draggedCanvasItem || pendingDuplicateCanvasSourceIds.length === 0) return
    const sourceItems = pendingDuplicateCanvasSourceIds
      .map(sourceId => options.messages.value.find(item => item.id === sourceId))
      .filter((item): item is CanvasImageItem => !!item)
    if (sourceItems.length === 0) return

    const clones = sourceItems.map(cloneCanvasItem)
    const cloneMap = new Map(sourceItems.map((item, index) => [item.id, clones[index]]))
    options.messages.value.push(...clones)
    options.selectedIds.value = clones.map(item => item.id)

    const activeClone = cloneMap.get(draggedCanvasItem.id) ?? clones[0]

    draggedCanvasItem = activeClone
    draggedSelectionOffsets = new Map(clones.map(item => [item.id, { x: item.x, y: item.y }]))
    initialX = activeClone.x
    initialY = activeClone.y
    pendingDuplicateCanvasSourceIds = []
  }

  function applyDuplicatedNodeSelection() {
    if (pendingDuplicateNodeKeys.length === 0) return

    const nextOffsets = new Map<string, { node: DraggableNode; x: number; y: number }>()
    const nextSelectionKeys: string[] = []
    let nextPrimaryKey: string | null = null

    pendingDuplicateNodeKeys.forEach((selectionKey) => {
      const originalNode = options.resolveNodeBySelectionKey(selectionKey)
      const [kind, rawId] = selectionKey.split(':')
      const id = Number(rawId)
      if (!originalNode || !kind || !Number.isFinite(id)) return

      const duplicate = cloneCanvasNode(originalNode)
      if (kind === 'image') options.imageNodes.value.push(duplicate as ImageNodeItem)
      else if (kind === 'prompt') options.promptNodes.value.push(duplicate as PromptNodeItem)
      else if (kind === 'model') options.modelNodes.value.push(duplicate as ModelNodeItem)
      else if (kind === 'mannequin') options.mannequinNodes.value.push(duplicate as MannequinNodeItem)
      else if (kind === 'viewpoint') options.viewpointNodes.value.push(duplicate as ViewpointNodeItem)
      else if (kind === 'mask') options.maskNodes.value.push(duplicate as MaskNodeItem)
      else if (kind === 'color') options.colorNodes.value.push(duplicate as ColorNodeItem)
      else if (kind === 'edit') options.editNodes.value.push(duplicate as EditNodeItem)
      else if (kind === 'fusion') options.fusionNodes.value.push(duplicate as FusionNodeItem)
      else if (kind === 'enhance') options.enhanceNodes.value.push(duplicate as EnhanceNodeItem)
      else if (kind === 'tryon') options.tryOnNodes.value.push(duplicate as TryOnNodeItem)
      else if (kind === 'faceswap') options.faceSwapNodes.value.push(duplicate as FaceSwapNodeItem)
      else options.generateNodes.value.push(duplicate as GenerateNodeItem)

      const duplicateKey = options.getNodeSelectionKey(kind as NodeDragKind, duplicate.id)
      nextSelectionKeys.push(duplicateKey)
      nextOffsets.set(duplicateKey, {
        node: duplicate as DraggableNode,
        x: duplicate.x,
        y: duplicate.y,
      })

      if (draggedNodePrimaryKey === selectionKey) {
        nextPrimaryKey = duplicateKey
        options.activateNodeByKind(kind as NodeDragKind, duplicate.id)
      }
    })

    if (!nextPrimaryKey && nextSelectionKeys.length > 0) {
      const [kind, rawId] = nextSelectionKeys[0].split(':')
      const id = Number(rawId)
      if (kind && Number.isFinite(id)) {
        options.activateNodeByKind(kind as NodeDragKind, id)
      }
      nextPrimaryKey = nextSelectionKeys[0]
    }

    options.selectedNodeKeys.value = nextSelectionKeys
    draggedNodeOffsets = nextOffsets
    draggedNodePrimaryKey = nextPrimaryKey
    pendingDuplicateNodeKeys = []
  }

  function cancelNodeDrag() {
    if (nodeDragRAF) {
      window.cancelAnimationFrame(nodeDragRAF)
      nodeDragRAF = null
    }
    draggedNodeOffsets.clear()
    draggedNodePrimaryKey = null
    options.draggingNodeKeys.value = []
  }

  function resetPointerUiState() {
    options.clearSelectedConnection()
    clearCanvasQuickActionState()
    options.clearActiveNodes()
    closeNodeContextMenu()
    options.isPanning.value = false
    options.isSelecting.value = false
    options.isDraggingItem.value = false
    options.isResizingItem.value = false
    options.selectionRect.value = { x1: 0, y1: 0, x2: 0, y2: 0 }
    cancelNodeDrag()
    clearPendingInteraction()
    marqueeBaseSelectionIds = []
    marqueeBaseNodeSelectionKeys = []
    endHistoryBatch(false)
    draggedCanvasItem = null
    draggedSelectionOffsets.clear()
    document.body.style.cursor = ''
  }

  function onCanvasContextMenu(e: MouseEvent) {
    const target = e.target as HTMLElement | null
    if (
      target?.closest('.floating-header') ||
      target?.closest('.floating-toolbar') ||
      target?.closest('.floating-bottom-bar') ||
      target?.closest('.floating-status') ||
      target?.closest('.node-context-menu') ||
      target?.closest('.workflow-node')
    ) {
      return
    }
    const rect = options.canvasLayoutRef.value?.getBoundingClientRect()
    if (!rect) return
    e.preventDefault()
    clearCanvasQuickActionState()
    options.nodeContextMenu.value = {
      visible: true,
      x: e.clientX - rect.left,
      y: e.clientY - rect.top,
      clientX: e.clientX,
      clientY: e.clientY,
    }
  }

  function onWindowMouseDown(e: MouseEvent) {
    const target = e.target as HTMLElement | null
    if (options.isBoardMenuOpen() && !target?.closest('.file-name-chip')) {
      options.setBoardMenuOpen(false)
    }
    if (options.isBoardRenaming() && !target?.closest('.file-name-chip')) {
      options.confirmBoardRename()
    }
    if (!options.nodeContextMenu.value.visible) return
    if (target?.closest('.node-context-menu')) return
    closeNodeContextMenu()
  }

  function onCanvasMouseDown(e: MouseEvent) {
    options.setBoardMenuOpen(false)
    if (options.isBoardRenaming()) options.confirmBoardRename()
    closeNodeContextMenu()
    options.clearSelectedConnection()
    options.clearActiveNodes()
    clearCanvasQuickActionState()
    if (!e.shiftKey) {
      options.clearSelectedNodeKeys()
    }

    if (e.button === 1 || options.isSpacePressed.value || options.activeTool.value === 'hand') {
      pendingInteraction = 'pan'
      startX = e.clientX
      startY = e.clientY
      initialX = options.canvasX.value
      initialY = options.canvasY.value
      e.preventDefault()
      return
    }

    if (e.button === 0 && options.activeTool.value === 'select') {
      pendingInteraction = 'select'
      startX = e.clientX
      startY = e.clientY
      options.selectionRect.value = { x1: startX, y1: startY, x2: startX, y2: startY }
      marqueeBaseSelectionIds = e.shiftKey ? [...options.selectedIds.value] : []
      marqueeBaseNodeSelectionKeys = e.shiftKey ? [...options.selectedNodeKeys.value] : []
      if (!e.shiftKey) {
        options.selectedIds.value = []
        options.clearSelectedNodeKeys()
      }
    }
  }

  function onItemMouseDown(e: MouseEvent, msg: CanvasImageItem) {
    if (options.activeTool.value !== 'select' || e.button !== 0) return
    closeNodeContextMenu()
    options.clearSelectedConnection()
    options.clearActiveNodes()
    clearCanvasQuickActionState()
    options.clearSelectedNodeKeys()

    if (e.shiftKey) {
      const idx = options.selectedIds.value.indexOf(msg.id)
      if (idx >= 0) options.selectedIds.value.splice(idx, 1)
      else options.selectedIds.value.push(msg.id)
      e.preventDefault()
      return
    } else if (!options.selectedIds.value.includes(msg.id)) {
      options.selectedIds.value = [msg.id]
    }

    pendingInteraction = 'item-drag'
    pendingDuplicateCanvasSourceIds = e.altKey
      ? (options.selectedIds.value.includes(msg.id) ? [...options.selectedIds.value] : [msg.id])
      : []
    draggedCanvasItem = msg
    draggedSelectionOffsets = new Map(
      options.selectedIds.value
        .map((selectedId) => {
          const selectedItem = options.messages.value.find(item => item.id === selectedId)
          return selectedItem ? [selectedId, { x: selectedItem.x, y: selectedItem.y }] as const : null
        })
        .filter((entry): entry is readonly [number, { x: number; y: number }] => entry !== null),
    )
    startX = e.clientX
    startY = e.clientY
    initialX = msg.x
    initialY = msg.y
    e.preventDefault()
  }

  function onItemContextMenu(_e: MouseEvent, msg: CanvasImageItem) {
    if (options.activeTool.value !== 'select' || msg.type !== 'image') return
    closeNodeContextMenu()
    options.clearSelectedConnection()
    options.clearSelectedNodeKeys()
    options.clearActiveNodes()
    clearCanvasQuickActionState()
    if (!options.selectedIds.value.includes(msg.id)) {
      options.selectedIds.value = [msg.id]
    }
    options.activeImageActionId.value = msg.id
  }

  function beginNodeDrag(kind: NodeDragKind, node: DraggableNode, e: MouseEvent) {
    if (options.activeTool.value !== 'select') return
    if (e.button !== 0) return
    closeNodeContextMenu()
    options.clearSelectedConnection()
    clearCanvasQuickActionState()
    options.selectedIds.value = []
    const selectionKey = options.getNodeSelectionKey(kind, node.id)
    const isModifierSelection = e.shiftKey || e.metaKey || e.ctrlKey

    if (isModifierSelection) {
      if (options.selectedNodeKeys.value.includes(selectionKey)) {
        options.removeSelectedNodeKey(kind, node.id)
        if (options.isNodeActive(kind, node.id)) {
          options.clearActiveNodes()
        }
      } else {
        options.selectedNodeKeys.value = [...options.selectedNodeKeys.value, selectionKey]
        options.activateNodeByKind(kind, node.id)
      }
      e.preventDefault()
      return
    }

    options.activateNodeByKind(kind, node.id)

    if (!options.selectedNodeKeys.value.includes(selectionKey)) {
      options.selectedNodeKeys.value = [selectionKey]
    }

    pendingInteraction = 'node-drag'
    const dragKeys = options.selectedNodeKeys.value.includes(selectionKey)
      ? [...options.selectedNodeKeys.value]
      : [selectionKey]
    pendingDuplicateNodeKeys = e.altKey ? [...dragKeys] : []
    draggedNodeOffsets = new Map(
      dragKeys
        .map((key) => {
          const targetNode = options.resolveNodeBySelectionKey(key)
          return targetNode ? [key, { node: targetNode, x: targetNode.x, y: targetNode.y }] as const : null
        })
        .filter((entry): entry is readonly [string, { node: DraggableNode; x: number; y: number }] => entry !== null),
    )
    draggedNodePrimaryKey = selectionKey
    startX = e.clientX
    startY = e.clientY
    initialX = node.x
    initialY = node.y
    e.preventDefault()
  }

  function onImageNodeMouseDown(e: MouseEvent, node: ImageNodeItem) {
    beginNodeDrag('image', node, e)
  }

  function onPromptNodeMouseDown(e: MouseEvent, node: PromptNodeItem) {
    beginNodeDrag('prompt', node, e)
  }

  function onModelNodeMouseDown(e: MouseEvent, node: ModelNodeItem) {
    beginNodeDrag('model', node, e)
  }

  function onMannequinNodeMouseDown(e: MouseEvent, node: MannequinNodeItem) {
    beginNodeDrag('mannequin', node, e)
  }

  function onViewpointNodeMouseDown(e: MouseEvent, node: ViewpointNodeItem) {
    beginNodeDrag('viewpoint', node, e)
  }

  function onMaskNodeMouseDown(e: MouseEvent, node: MaskNodeItem) {
    beginNodeDrag('mask', node, e)
  }

  function onColorNodeMouseDown(e: MouseEvent, node: ColorNodeItem) {
    beginNodeDrag('color', node, e)
  }

  function onEditNodeMouseDown(e: MouseEvent, node: EditNodeItem) {
    beginNodeDrag('edit', node, e)
  }

  function onFusionNodeMouseDown(e: MouseEvent, node: FusionNodeItem) {
    beginNodeDrag('fusion', node, e)
  }

  function onEnhanceNodeMouseDown(e: MouseEvent, node: EnhanceNodeItem) {
    beginNodeDrag('enhance', node, e)
  }

  function onTryOnNodeMouseDown(e: MouseEvent, node: TryOnNodeItem) {
    beginNodeDrag('tryon', node, e)
  }

  function onFaceSwapNodeMouseDown(e: MouseEvent, node: FaceSwapNodeItem) {
    beginNodeDrag('faceswap', node, e)
  }

  function onGenerateNodeMouseDown(e: MouseEvent, node: GenerateNodeItem) {
    beginNodeDrag('generate', node, e)
  }

  function applyNodeDragFrame() {
    nodeDragRAF = null
    if (draggedNodeOffsets.size === 0 || options.draggingNodeKeys.value.length === 0) return
    const zoom = options.zoomLevel.value / 100
    const dx = (nodeDragClientX - startX) / zoom
    const dy = (nodeDragClientY - startY) / zoom
    draggedNodeOffsets.forEach(({ node, x, y }) => {
      node.x = x + dx
      node.y = y + dy
    })
  }

  function onResizeMouseDown(e: MouseEvent, msg: CanvasImageItem, dir: string) {
    pendingInteraction = 'resize'
    draggedCanvasItem = msg
    resizeDir = dir
    startX = e.clientX
    startY = e.clientY

    const handleEl = e.currentTarget as HTMLElement | null
    const parent = handleEl?.closest('.canvas-item') as HTMLElement | null
    if (parent) {
      initialW = parent.offsetWidth
      initialH = parent.offsetHeight
    } else {
      initialW = msg.width || 300
      initialH = initialW
    }

    initialX = msg.x
    initialY = msg.y
    e.preventDefault()
  }

  function activatePendingInteractionIfNeeded(e: MouseEvent): boolean {
    if (!pendingInteraction) return false
    if (!hasExceededPointerThreshold(e.clientX, e.clientY)) return true

    const interaction = pendingInteraction
    pendingInteraction = null

    if (interaction === 'pan') {
      options.isPanning.value = true
      document.body.style.cursor = 'grabbing'
      beginHistoryBatch()
      return false
    }

    if (interaction === 'select') {
      options.isSelecting.value = true
      return false
    }

    if (interaction === 'item-drag') {
      const isDuplicateDrag = pendingDuplicateCanvasSourceIds.length > 0
      applyDuplicatedCanvasSelection()
      options.isDraggingItem.value = true
      beginHistoryBatch()
      document.body.style.cursor = isDuplicateDrag ? 'copy' : 'grabbing'
      return false
    }

    if (interaction === 'resize') {
      options.isResizingItem.value = true
      beginHistoryBatch()
      return false
    }

    if (interaction === 'node-drag' && draggedNodeOffsets.size > 0) {
      const isDuplicateDrag = pendingDuplicateNodeKeys.length > 0
      applyDuplicatedNodeSelection()
      options.draggingNodeKeys.value = Array.from(draggedNodeOffsets.keys())
      document.body.style.cursor = isDuplicateDrag ? 'copy' : 'grabbing'
      beginHistoryBatch()
      return false
    }

    return false
  }

  function onGlobalMouseMove(e: MouseEvent) {
    if (activatePendingInteractionIfNeeded(e)) return

    if (options.isPanning.value) {
      options.canvasX.value = initialX + (e.clientX - startX)
      options.canvasY.value = initialY + (e.clientY - startY)
      return
    }

    if (options.isSelecting.value) {
      options.selectionRect.value.x2 = e.clientX
      options.selectionRect.value.y2 = e.clientY
      return
    }

    if (options.draggingNodeKeys.value.length > 0 && draggedNodeOffsets.size > 0) {
      nodeDragClientX = e.clientX
      nodeDragClientY = e.clientY
      if (!nodeDragRAF) {
        nodeDragRAF = window.requestAnimationFrame(applyNodeDragFrame)
      }
      return
    }

    if (options.isDraggingItem.value && draggedCanvasItem) {
      const zoom = options.zoomLevel.value / 100
      const dx = (e.clientX - startX) / zoom
      const dy = (e.clientY - startY) / zoom
      const shouldMoveSelectionGroup =
        draggedSelectionOffsets.size > 1 && draggedSelectionOffsets.has(draggedCanvasItem.id)

      if (shouldMoveSelectionGroup) {
        draggedSelectionOffsets.forEach((origin, selectedId) => {
          const selectedItem = options.messages.value.find(item => item.id === selectedId)
          if (!selectedItem) return
          selectedItem.x = origin.x + dx
          selectedItem.y = origin.y + dy
        })
      } else {
        draggedCanvasItem.x = initialX + dx
        draggedCanvasItem.y = initialY + dy
      }
      return
    }

    if (options.isResizingItem.value && draggedCanvasItem) {
      const zoom = options.zoomLevel.value / 100
      const dx = (e.clientX - startX) / zoom
      const dy = (e.clientY - startY) / zoom
      const ratio = initialH / initialW
      let newW = initialW

      if (resizeDir.includes('e')) {
        newW = Math.max(50, initialW + dx)
      } else if (resizeDir.includes('w')) {
        newW = Math.max(50, initialW - dx)
        draggedCanvasItem.x = initialX + (initialW - newW)
      } else if (resizeDir.includes('n')) {
        newW = Math.max(50, initialW - dy / ratio)
      } else if (resizeDir.includes('s')) {
        newW = Math.max(50, initialW + dy / ratio)
      }

      if (resizeDir.includes('n')) {
        const deltaHeight = (newW - initialW) * ratio
        draggedCanvasItem.y = initialY - deltaHeight
      }

      draggedCanvasItem.width = newW
    }
  }

  function onGlobalMouseUp(e: MouseEvent) {
    if (options.isSelecting.value) {
      const xMin = Math.min(options.selectionRect.value.x1, options.selectionRect.value.x2)
      const xMax = Math.max(options.selectionRect.value.x1, options.selectionRect.value.x2)
      const yMin = Math.min(options.selectionRect.value.y1, options.selectionRect.value.y2)
      const yMax = Math.max(options.selectionRect.value.y1, options.selectionRect.value.y2)
      const hitIds: number[] = []
      const hitNodeKeys: string[] = []

      const rect = options.canvasLayoutRef.value?.getBoundingClientRect()
      if (rect) {
        const zoom = options.zoomLevel.value / 100
        options.messages.value.forEach((msg) => {
          const screenX = rect.left + rect.width / 2 + (options.canvasX.value + msg.x * zoom)
          const screenY = rect.top + rect.height / 2 + (options.canvasY.value + msg.y * zoom)
          const screenW = (msg.width || 300) * zoom
          const screenH = screenW * (msg.height_ratio || 1)

          if (screenX + screenW > xMin && screenX < xMax && screenY + screenH > yMin && screenY < yMax) {
            hitIds.push(msg.id)
          }
        })

        options.canvasLayoutRef.value
          ?.querySelectorAll<HTMLElement>('.workflow-node[data-node-kind][data-node-id]')
          .forEach((element) => {
            const kind = element.dataset.nodeKind as NodeDragKind | undefined
            const rawId = element.dataset.nodeId
            if (!kind || !rawId) return
            const id = Number(rawId)
            if (!Number.isFinite(id)) return
            const nodeRect = element.getBoundingClientRect()
            if (
              nodeRect.right > xMin &&
              nodeRect.left < xMax &&
              nodeRect.bottom > yMin &&
              nodeRect.top < yMax
            ) {
              hitNodeKeys.push(options.getNodeSelectionKey(kind, id))
            }
          })
      }

      options.selectedIds.value = marqueeBaseSelectionIds.length > 0
        ? Array.from(new Set([...marqueeBaseSelectionIds, ...hitIds]))
        : hitIds
      options.selectedNodeKeys.value = marqueeBaseNodeSelectionKeys.length > 0
        ? Array.from(new Set([...marqueeBaseNodeSelectionKeys, ...hitNodeKeys]))
        : hitNodeKeys
    }

    if (options.isDraggingItem.value && draggedCanvasItem?.type === 'image' && draggedSelectionOffsets.size <= 1) {
      const importedToImageNode = options.importCanvasImageToImageNodeAtPoint(e.clientX, e.clientY, draggedCanvasItem)
      const importedToMask = importedToImageNode
        ? false
        : options.importCanvasImageToMaskNodeAtPoint(e.clientX, e.clientY, draggedCanvasItem)
      const importedToModel = importedToImageNode || importedToMask
        ? false
        : options.importCanvasImageToModelNodeAtPoint(e.clientX, e.clientY, draggedCanvasItem)
      const importedToMannequin = importedToImageNode || importedToMask || importedToModel
        ? false
        : options.importCanvasImageToMannequinNodeAtPoint(e.clientX, e.clientY, draggedCanvasItem)
      const importedToViewpoint = importedToImageNode || importedToMask || importedToModel || importedToMannequin
        ? false
        : options.importCanvasImageToViewpointNodeAtPoint(e.clientX, e.clientY, draggedCanvasItem)

      if (importedToImageNode || importedToMask || importedToModel || importedToMannequin || importedToViewpoint) {
        draggedCanvasItem.x = initialX
        draggedCanvasItem.y = initialY
        if (importedToImageNode) {
          options.message.success('已导入 Image 节点')
        } else if (importedToMannequin) {
          options.message.success('已导入人台节点')
        } else if (importedToViewpoint) {
          options.message.success('已导入 Viewpoint 节点')
        } else {
          options.message.success(importedToMask ? '已导入 Mask 节点' : '已导入为模特图')
        }
      }
    }

    options.isPanning.value = false
    options.isDraggingItem.value = false
    options.isResizingItem.value = false
    options.isSelecting.value = false
    clearPendingInteraction()
    marqueeBaseSelectionIds = []
    marqueeBaseNodeSelectionKeys = []
    endHistoryBatch(true)
    draggedCanvasItem = null
    draggedSelectionOffsets.clear()
    cancelNodeDrag()
    document.body.style.cursor = ''
  }

  onMounted(() => {
    window.addEventListener('mousedown', onWindowMouseDown)
    window.addEventListener('mousemove', onGlobalMouseMove)
    window.addEventListener('mouseup', onGlobalMouseUp)
  })

  onUnmounted(() => {
    cancelNodeDrag()
    window.removeEventListener('mousedown', onWindowMouseDown)
    window.removeEventListener('mousemove', onGlobalMouseMove)
    window.removeEventListener('mouseup', onGlobalMouseUp)
  })

  return {
    closeNodeContextMenu,
    resetPointerUiState,
    onCanvasContextMenu,
    onCanvasMouseDown,
    onItemMouseDown,
    onItemContextMenu,
    onImageNodeMouseDown,
    onPromptNodeMouseDown,
    onModelNodeMouseDown,
    onMannequinNodeMouseDown,
    onViewpointNodeMouseDown,
    onMaskNodeMouseDown,
    onColorNodeMouseDown,
    onEditNodeMouseDown,
    onFusionNodeMouseDown,
    onEnhanceNodeMouseDown,
    onTryOnNodeMouseDown,
    onFaceSwapNodeMouseDown,
    onGenerateNodeMouseDown,
    onResizeMouseDown,
  }
}
