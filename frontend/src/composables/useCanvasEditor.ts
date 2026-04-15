import { computed, onMounted, onUnmounted, ref, type Ref } from 'vue'
import type {
  CanvasImageItem,
  CanvasPoint,
  CanvasNodeItem,
  ColorNodeItem,
  ImageNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PromptNodeItem,
} from '../types/canvasNodes'

interface MessageApiLike {
  success(content: string): unknown
}

interface UseCanvasEditorOptions {
  messages: Ref<CanvasImageItem[]>
  imageNodes: Ref<ImageNodeItem[]>
  promptNodes: Ref<PromptNodeItem[]>
  modelNodes: Ref<ModelNodeItem[]>
  maskNodes: Ref<MaskNodeItem[]>
  colorNodes: Ref<ColorNodeItem[]>
  activeTool: Ref<string>
  showStyleStrengthControl: Ref<boolean>
  canvasX: Ref<number>
  canvasY: Ref<number>
  zoomLevel: Ref<number>
  canvasLayoutRef: Ref<HTMLElement | null>
  isBoardMenuOpen: () => boolean
  setBoardMenuOpen: (visible: boolean) => void
  isBoardRenaming: () => boolean
  isLeaveConfirmVisible: () => boolean
  activeColorPopoverNodeId: () => number | null
  confirmBoardRename: () => void
  resolveLeaveConfirm: (decision: 'save' | 'discard' | 'cancel') => void
  closeColorPopover: () => void
  updateZoom: (delta: number, anchor?: { clientX: number; clientY: number }) => void
  getCanvasPointFromClient: (clientX: number, clientY: number) => CanvasPoint
  createCanvasImageAt: (point: CanvasPoint, url: string, ratio?: number) => CanvasImageItem
  uploadImage: (file: File) => Promise<{ code?: number; data?: { url?: string } }>
  importCanvasImageToImageNodeAtPoint: (clientX: number, clientY: number, imageItem: CanvasImageItem) => boolean
  importCanvasImageToMaskNodeAtPoint: (clientX: number, clientY: number, imageItem: CanvasImageItem) => boolean
  importCanvasImageToModelNodeAtPoint: (clientX: number, clientY: number, imageItem: CanvasImageItem) => boolean
  deleteImageNode: (id: number) => void
  deletePromptNode: (id: number) => void
  deleteModelNode: (id: number) => void
  deleteMaskNode: (id: number) => void
  deleteColorNode: (id: number) => void
  beginHistoryBatch: () => void
  endHistoryBatch: (commit?: boolean) => void
  undo: () => Promise<boolean> | boolean
  redo: () => Promise<boolean> | boolean
  message: MessageApiLike
  nodeImageDragMime: string
}

type NodeDragKind = 'image' | 'prompt' | 'model' | 'mask' | 'color'
type DraggableNode = ImageNodeItem | PromptNodeItem | ModelNodeItem | MaskNodeItem | ColorNodeItem
type PendingInteraction = 'pan' | 'select' | 'item-drag' | 'node-drag' | 'resize'

const POINTER_INTERACTION_THRESHOLD = 4

function createCanvasEntityId(): number {
  return Date.now() + Math.random()
}

function isInputLike(target: EventTarget | null): target is HTMLInputElement | HTMLTextAreaElement {
  return target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (isInputLike(target)) return true
  if (!(target instanceof HTMLElement)) return false
  return !!target.closest('input, textarea, [contenteditable="true"]')
}

export function useCanvasEditor(options: UseCanvasEditorOptions) {
  const selectedIds = ref<number[]>([])
  const selectedNodeKeys = ref<string[]>([])
  const activeImageActionId = ref<number | null>(null)
  const isDuplicateModifierPressed = ref(false)

  const activeImageNodeId = ref<number | null>(null)
  const activePromptNodeId = ref<number | null>(null)
  const activeModelNodeId = ref<number | null>(null)
  const activeMaskNodeId = ref<number | null>(null)
  const activeColorNodeId = ref<number | null>(null)

  const nodeContextMenu = ref({
    visible: false,
    x: 0,
    y: 0,
    clientX: 0,
    clientY: 0,
  })

  const isPanning = ref(false)
  const isSelecting = ref(false)
  const isDraggingItem = ref(false)
  const draggingNodeKeys = ref<string[]>([])
  const isResizingItem = ref(false)
  const isSpacePressed = ref(false)
  const selectionRect = ref({ x1: 0, y1: 0, x2: 0, y2: 0 })

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

  function clearActiveNodes() {
    activeImageNodeId.value = null
    activePromptNodeId.value = null
    activeModelNodeId.value = null
    activeMaskNodeId.value = null
    activeColorNodeId.value = null
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
      kind !== 'mask' &&
      kind !== 'color'
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
    if (kind === 'mask') return options.maskNodes.value.find(node => node.id === id) ?? null
    return options.colorNodes.value.find(node => node.id === id) ?? null
  }

  function isNodeSelected(kind: NodeDragKind, id: number): boolean {
    const key = getNodeSelectionKey(kind, id)
    if (selectedNodeKeys.value.includes(key)) return true
    if (kind === 'image') return activeImageNodeId.value === id
    if (kind === 'prompt') return activePromptNodeId.value === id
    if (kind === 'model') return activeModelNodeId.value === id
    if (kind === 'mask') return activeMaskNodeId.value === id
    return activeColorNodeId.value === id
  }

  function isNodeActive(kind: NodeDragKind, id: number): boolean {
    if (kind === 'image') return activeImageNodeId.value === id
    if (kind === 'prompt') return activePromptNodeId.value === id
    if (kind === 'model') return activeModelNodeId.value === id
    if (kind === 'mask') return activeMaskNodeId.value === id
    return activeColorNodeId.value === id
  }

  function isNodeDragging(kind: NodeDragKind, id: number): boolean {
    return draggingNodeKeys.value.includes(getNodeSelectionKey(kind, id))
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

  function activateMaskNode(id: number) {
    clearActiveNodes()
    activeMaskNodeId.value = id
  }

  function activateColorNode(id: number) {
    clearActiveNodes()
    activeColorNodeId.value = id
  }

  function activateNodeByKind(kind: NodeDragKind, id: number) {
    if (kind === 'image') activateImageNode(id)
    else if (kind === 'prompt') activatePromptNode(id)
    else if (kind === 'model') activateModelNode(id)
    else if (kind === 'mask') activateMaskNode(id)
    else activateColorNode(id)
  }

  function closeNodeContextMenu() {
    if (!nodeContextMenu.value.visible) return
    nodeContextMenu.value.visible = false
  }

  function clearCanvasQuickActionState() {
    activeImageActionId.value = null
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
    return {
      ...node,
      id: createCanvasEntityId(),
    }
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
    selectedIds.value = clones.map(item => item.id)

    const activeClone = cloneMap.get(draggedCanvasItem.id)
      ?? clones[0]

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
      const originalNode = resolveNodeBySelectionKey(selectionKey)
      const parsed = parseNodeSelectionKey(selectionKey)
      if (!originalNode || !parsed) return

      const duplicate = cloneCanvasNode(originalNode)
      if (parsed.kind === 'image') options.imageNodes.value.push(duplicate as ImageNodeItem)
      else if (parsed.kind === 'prompt') options.promptNodes.value.push(duplicate as PromptNodeItem)
      else if (parsed.kind === 'model') options.modelNodes.value.push(duplicate as ModelNodeItem)
      else if (parsed.kind === 'mask') options.maskNodes.value.push(duplicate as MaskNodeItem)
      else options.colorNodes.value.push(duplicate as ColorNodeItem)

      const duplicateKey = getNodeSelectionKey(parsed.kind, duplicate.id)
      nextSelectionKeys.push(duplicateKey)
      nextOffsets.set(duplicateKey, {
        node: duplicate as DraggableNode,
        x: duplicate.x,
        y: duplicate.y,
      })

      if (draggedNodePrimaryKey === selectionKey) {
        nextPrimaryKey = duplicateKey
        activateNodeByKind(parsed.kind, duplicate.id)
      }
    })

    if (!nextPrimaryKey && nextSelectionKeys.length > 0) {
      const parsed = parseNodeSelectionKey(nextSelectionKeys[0])
      if (parsed) activateNodeByKind(parsed.kind, parsed.id)
      nextPrimaryKey = nextSelectionKeys[0]
    }

    selectedNodeKeys.value = nextSelectionKeys
    draggedNodeOffsets = nextOffsets
    draggedNodePrimaryKey = nextPrimaryKey
    pendingDuplicateNodeKeys = []
  }

  function resetEditorUiState() {
    selectedIds.value = []
    clearSelectedNodeKeys()
    clearCanvasQuickActionState()
    clearActiveNodes()
    closeNodeContextMenu()
    options.closeColorPopover()
    isPanning.value = false
    isSelecting.value = false
    isDraggingItem.value = false
    isResizingItem.value = false
    selectionRect.value = { x1: 0, y1: 0, x2: 0, y2: 0 }
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
    options.closeColorPopover()
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
    nodeContextMenu.value = {
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
    if (options.activeColorPopoverNodeId() !== null && !target?.closest('.color-node-color-wrap')) {
      options.closeColorPopover()
    }
    if (!nodeContextMenu.value.visible) return
    if (target?.closest('.node-context-menu')) return
    closeNodeContextMenu()
  }

  function onCanvasMouseDown(e: MouseEvent) {
    options.setBoardMenuOpen(false)
    if (options.isBoardRenaming()) options.confirmBoardRename()
    options.closeColorPopover()
    closeNodeContextMenu()
    clearActiveNodes()
    clearCanvasQuickActionState()
    if (!e.shiftKey) {
      clearSelectedNodeKeys()
    }

    if (e.button === 1 || isSpacePressed.value || options.activeTool.value === 'hand') {
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
      selectionRect.value = { x1: startX, y1: startY, x2: startX, y2: startY }
      marqueeBaseSelectionIds = e.shiftKey ? [...selectedIds.value] : []
      marqueeBaseNodeSelectionKeys = e.shiftKey ? [...selectedNodeKeys.value] : []
      if (!e.shiftKey) {
        selectedIds.value = []
        clearSelectedNodeKeys()
      }
    }
  }

  function onItemMouseDown(e: MouseEvent, msg: CanvasImageItem) {
    if (options.activeTool.value !== 'select' || e.button !== 0) return
    options.closeColorPopover()
    closeNodeContextMenu()
    clearActiveNodes()
    clearCanvasQuickActionState()
    clearSelectedNodeKeys()

    if (e.shiftKey) {
      const idx = selectedIds.value.indexOf(msg.id)
      if (idx >= 0) selectedIds.value.splice(idx, 1)
      else selectedIds.value.push(msg.id)
      e.preventDefault()
      return
    } else if (!selectedIds.value.includes(msg.id)) {
      selectedIds.value = [msg.id]
    }

    pendingInteraction = 'item-drag'
    pendingDuplicateCanvasSourceIds = e.altKey
      ? (selectedIds.value.includes(msg.id) ? [...selectedIds.value] : [msg.id])
      : []
    draggedCanvasItem = msg
    draggedSelectionOffsets = new Map(
      selectedIds.value
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
    options.closeColorPopover()
    closeNodeContextMenu()
    clearActiveNodes()
    clearSelectedNodeKeys()
    options.showStyleStrengthControl.value = false
    selectedIds.value = [msg.id]
    activeImageActionId.value = msg.id
  }

  function beginNodeDrag(kind: NodeDragKind, node: DraggableNode, e: MouseEvent) {
    if (e.button !== 0) return
    options.closeColorPopover()
    closeNodeContextMenu()
    clearCanvasQuickActionState()
    selectedIds.value = []
    const selectionKey = getNodeSelectionKey(kind, node.id)
    const isModifierSelection = e.shiftKey || e.metaKey || e.ctrlKey

    if (isModifierSelection) {
      if (selectedNodeKeys.value.includes(selectionKey)) {
        removeSelectedNodeKey(kind, node.id)
        if (isNodeActive(kind, node.id)) {
          clearActiveNodes()
        }
      } else {
        selectedNodeKeys.value = [...selectedNodeKeys.value, selectionKey]
        activateNodeByKind(kind, node.id)
      }
      e.preventDefault()
      return
    }

    activateNodeByKind(kind, node.id)

    if (!selectedNodeKeys.value.includes(selectionKey)) {
      selectedNodeKeys.value = [selectionKey]
    }

    pendingInteraction = 'node-drag'
    const dragKeys = selectedNodeKeys.value.includes(selectionKey)
      ? [...selectedNodeKeys.value]
      : [selectionKey]
    pendingDuplicateNodeKeys = e.altKey ? [...dragKeys] : []
    draggedNodeOffsets = new Map(
      dragKeys
        .map((key) => {
          const targetNode = resolveNodeBySelectionKey(key)
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

  function onMaskNodeMouseDown(e: MouseEvent, node: MaskNodeItem) {
    beginNodeDrag('mask', node, e)
  }

  function onColorNodeMouseDown(e: MouseEvent, node: ColorNodeItem) {
    beginNodeDrag('color', node, e)
  }

  function applyNodeDragFrame() {
    nodeDragRAF = null
    if (draggedNodeOffsets.size === 0 || draggingNodeKeys.value.length === 0) return
    const zoom = options.zoomLevel.value / 100
    const dx = (nodeDragClientX - startX) / zoom
    const dy = (nodeDragClientY - startY) / zoom
    draggedNodeOffsets.forEach(({ node, x, y }) => {
      node.x = x + dx
      node.y = y + dy
    })
  }

  function cancelNodeDrag() {
    if (nodeDragRAF) {
      window.cancelAnimationFrame(nodeDragRAF)
      nodeDragRAF = null
    }
    draggedNodeOffsets.clear()
    draggedNodePrimaryKey = null
    draggingNodeKeys.value = []
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

  function readNodeImageDropPayload(e: DragEvent): { url: string; ratio: number } | null {
    const rawUrl = e.dataTransfer?.getData(options.nodeImageDragMime)
    if (!rawUrl) return null
    const url = rawUrl.trim()
    if (!url) return null
    const rawRatio = e.dataTransfer?.getData(`${options.nodeImageDragMime}-ratio`) || ''
    const ratio = Number(rawRatio)
    return {
      url,
      ratio: Number.isFinite(ratio) && ratio > 0 ? ratio : 1,
    }
  }

  function activatePendingInteractionIfNeeded(e: MouseEvent): boolean {
    if (!pendingInteraction) return false
    if (!hasExceededPointerThreshold(e.clientX, e.clientY)) return true

    const interaction = pendingInteraction
    pendingInteraction = null

    if (interaction === 'pan') {
      isPanning.value = true
      document.body.style.cursor = 'grabbing'
      beginHistoryBatch()
      return false
    }

    if (interaction === 'select') {
      isSelecting.value = true
      return false
    }

    if (interaction === 'item-drag') {
      const isDuplicateDrag = pendingDuplicateCanvasSourceIds.length > 0
      applyDuplicatedCanvasSelection()
      isDraggingItem.value = true
      beginHistoryBatch()
      document.body.style.cursor = isDuplicateDrag ? 'copy' : 'grabbing'
      return false
    }

    if (interaction === 'resize') {
      isResizingItem.value = true
      beginHistoryBatch()
      return false
    }

    if (interaction === 'node-drag' && draggedNodeOffsets.size > 0) {
      const isDuplicateDrag = pendingDuplicateNodeKeys.length > 0
      applyDuplicatedNodeSelection()
      draggingNodeKeys.value = Array.from(draggedNodeOffsets.keys())
      document.body.style.cursor = isDuplicateDrag ? 'copy' : 'grabbing'
      beginHistoryBatch()
      return false
    }

    return false
  }

  function onGlobalMouseMove(e: MouseEvent) {
    if (activatePendingInteractionIfNeeded(e)) return

    if (isPanning.value) {
      options.canvasX.value = initialX + (e.clientX - startX)
      options.canvasY.value = initialY + (e.clientY - startY)
      return
    }

    if (isSelecting.value) {
      selectionRect.value.x2 = e.clientX
      selectionRect.value.y2 = e.clientY
      return
    }

    if (draggingNodeKeys.value.length > 0 && draggedNodeOffsets.size > 0) {
      nodeDragClientX = e.clientX
      nodeDragClientY = e.clientY
      if (!nodeDragRAF) {
        nodeDragRAF = window.requestAnimationFrame(applyNodeDragFrame)
      }
      return
    }

    if (isDraggingItem.value && draggedCanvasItem) {
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

    if (isResizingItem.value && draggedCanvasItem) {
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
    if (isSelecting.value) {
      const xMin = Math.min(selectionRect.value.x1, selectionRect.value.x2)
      const xMax = Math.max(selectionRect.value.x1, selectionRect.value.x2)
      const yMin = Math.min(selectionRect.value.y1, selectionRect.value.y2)
      const yMax = Math.max(selectionRect.value.y1, selectionRect.value.y2)
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
              hitNodeKeys.push(getNodeSelectionKey(kind, id))
            }
          })
      }

      selectedIds.value = marqueeBaseSelectionIds.length > 0
        ? Array.from(new Set([...marqueeBaseSelectionIds, ...hitIds]))
        : hitIds
      selectedNodeKeys.value = marqueeBaseNodeSelectionKeys.length > 0
        ? Array.from(new Set([...marqueeBaseNodeSelectionKeys, ...hitNodeKeys]))
        : hitNodeKeys
    }

    if (isDraggingItem.value && draggedCanvasItem?.type === 'image' && draggedSelectionOffsets.size <= 1) {
      const importedToImageNode = options.importCanvasImageToImageNodeAtPoint(e.clientX, e.clientY, draggedCanvasItem)
      const importedToMask = importedToImageNode
        ? false
        : options.importCanvasImageToMaskNodeAtPoint(e.clientX, e.clientY, draggedCanvasItem)
      const importedToModel = importedToImageNode || importedToMask
        ? false
        : options.importCanvasImageToModelNodeAtPoint(e.clientX, e.clientY, draggedCanvasItem)

      if (importedToImageNode || importedToMask || importedToModel) {
        draggedCanvasItem.x = initialX
        draggedCanvasItem.y = initialY
        if (importedToImageNode) {
          options.message.success('已导入 Image 节点')
        } else {
          options.message.success(importedToMask ? '已导入 Mask 节点' : '已导入为模特图')
        }
      }
    }

    isPanning.value = false
    isDraggingItem.value = false
    isResizingItem.value = false
    isSelecting.value = false
    clearPendingInteraction()
    marqueeBaseSelectionIds = []
    marqueeBaseNodeSelectionKeys = []
    endHistoryBatch(true)
    draggedCanvasItem = null
    draggedSelectionOffsets.clear()
    cancelNodeDrag()
    document.body.style.cursor = ''
  }

  function handleKey(e: KeyboardEvent) {
    if (options.isLeaveConfirmVisible()) {
      if (e.type === 'keydown' && e.key === 'Escape') {
        options.resolveLeaveConfirm('cancel')
      }
      return
    }

    if (e.type === 'keydown' && e.key === 'Escape') {
      if (isEditableTarget(e.target)) return
      e.preventDefault()

      if (nodeContextMenu.value.visible) {
        closeNodeContextMenu()
        return
      }
      if (options.activeColorPopoverNodeId() !== null) {
        options.closeColorPopover()
        return
      }
      if (activeImageActionId.value !== null || options.showStyleStrengthControl.value) {
        clearCanvasQuickActionState()
        return
      }
      if (
        activeImageNodeId.value !== null ||
        activePromptNodeId.value !== null ||
        activeModelNodeId.value !== null ||
        activeMaskNodeId.value !== null ||
        activeColorNodeId.value !== null ||
        selectedNodeKeys.value.length > 0
      ) {
        clearSelectedNodeKeys()
        clearActiveNodes()
        return
      }
      if (selectedIds.value.length > 0) {
        clearSelectedIds()
      }
      return
    }

    if (
      e.type === 'keydown' &&
      (e.metaKey || e.ctrlKey) &&
      e.shiftKey &&
      e.key.toLowerCase() === 'z'
    ) {
      if (isEditableTarget(e.target)) return
      e.preventDefault()
      void options.redo()
      return
    }

    if (
      e.type === 'keydown' &&
      (e.metaKey || e.ctrlKey) &&
      !e.shiftKey &&
      e.key.toLowerCase() === 'z'
    ) {
      if (isEditableTarget(e.target)) return
      e.preventDefault()
      void options.undo()
      return
    }

    if (e.code === 'Space') {
      isSpacePressed.value = e.type === 'keydown'
    }

    if (e.key === 'Alt') {
      isDuplicateModifierPressed.value = e.type === 'keydown'
    }

    if (e.key === 'Delete' || e.key === 'Backspace') {
      if (isEditableTarget(e.target)) return
      if (deleteActiveNode()) {
        closeNodeContextMenu()
        clearCanvasQuickActionState()
        return
      }
      if (selectedIds.value.length > 0) {
        options.messages.value = options.messages.value.filter(item => !selectedIds.value.includes(item.id))
        selectedIds.value = []
        clearCanvasQuickActionState()
      }
    }
  }

  function handleWheel(e: WheelEvent) {
    const target = e.target as HTMLElement | null
    if (!target || !target.closest('.canvas-layout')) return
    const wheelPanSpeed = 1.5
    closeNodeContextMenu()

    if (target.closest('.settings-panel') || target.closest('.selected-preview-list')) return

    if (e.altKey) {
      e.preventDefault()
      options.updateZoom(e.deltaY > 0 ? -5 : 5, { clientX: e.clientX, clientY: e.clientY })
      return
    }

    if (e.ctrlKey || e.metaKey) {
      e.preventDefault()
      const horizontalDelta = (Math.abs(e.deltaX) > 0 ? e.deltaX : e.deltaY) * wheelPanSpeed
      options.canvasX.value -= horizontalDelta
      return
    }

    e.preventDefault()
    options.canvasX.value -= e.deltaX * wheelPanSpeed
    options.canvasY.value -= e.deltaY * wheelPanSpeed
  }

  async function handleDrop(e: DragEvent) {
    e.preventDefault()
    const payload = readNodeImageDropPayload(e)
    if (payload) {
      const point = options.getCanvasPointFromClient(e.clientX, e.clientY)
      const newItem = options.createCanvasImageAt(point, payload.url, payload.ratio)
      options.messages.value.push(newItem)
      selectedIds.value = [newItem.id]
      return
    }

    const files = e.dataTransfer?.files
    if (!files || !files[0]) return
    const { x, y } = options.getCanvasPointFromClient(e.clientX, e.clientY)

    for (const file of Array.from(files)) {
      if (!file.type.startsWith('image/')) continue
      try {
        const res = await options.uploadImage(file)
        if (res.code === 0 && res.data?.url) {
          options.messages.value.push(options.createCanvasImageAt({ x, y }, res.data.url))
        }
      } catch {
        // Keep drop interaction resilient; upload failure is already surfaced elsewhere.
      }
    }
  }

  function handleSelectStart(e: Event) {
    const target = e.target as HTMLElement | null
    if (!target?.closest('.canvas-layout')) return
    if (isEditableTarget(target)) return
    e.preventDefault()
  }

  function handleWindowBlur() {
    isSpacePressed.value = false
    isDuplicateModifierPressed.value = false
  }

  function clearSelectedIds() {
    selectedIds.value = []
    clearSelectedNodeKeys()
    clearCanvasQuickActionState()
    clearActiveNodes()
  }

  function deleteSelectedNodes(): boolean {
    if (selectedNodeKeys.value.length === 0) return false

    selectedNodeKeys.value.forEach((selectionKey) => {
      const parsed = parseNodeSelectionKey(selectionKey)
      if (!parsed) return
      if (parsed.kind === 'image') options.deleteImageNode(parsed.id)
      else if (parsed.kind === 'prompt') options.deletePromptNode(parsed.id)
      else if (parsed.kind === 'model') options.deleteModelNode(parsed.id)
      else if (parsed.kind === 'mask') options.deleteMaskNode(parsed.id)
      else options.deleteColorNode(parsed.id)
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
    if (activeMaskNodeId.value !== null) {
      options.deleteMaskNode(activeMaskNodeId.value)
      return true
    }
    if (activeColorNodeId.value !== null) {
      options.deleteColorNode(activeColorNodeId.value)
      return true
    }
    return false
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

  onMounted(() => {
    window.addEventListener('wheel', handleWheel, { passive: false })
    window.addEventListener('mousedown', onWindowMouseDown)
    window.addEventListener('keydown', handleKey)
    window.addEventListener('keyup', handleKey)
    window.addEventListener('mousemove', onGlobalMouseMove)
    window.addEventListener('mouseup', onGlobalMouseUp)
    window.addEventListener('selectstart', handleSelectStart)
    window.addEventListener('blur', handleWindowBlur)
  })

  onUnmounted(() => {
    cancelNodeDrag()
    window.removeEventListener('wheel', handleWheel)
    window.removeEventListener('mousedown', onWindowMouseDown)
    window.removeEventListener('keydown', handleKey)
    window.removeEventListener('keyup', handleKey)
    window.removeEventListener('mousemove', onGlobalMouseMove)
    window.removeEventListener('mouseup', onGlobalMouseUp)
    window.removeEventListener('selectstart', handleSelectStart)
    window.removeEventListener('blur', handleWindowBlur)
  })

  return {
    selectedIds,
    selectedNodeKeys,
    activeImageActionId,
    isDuplicateModifierPressed,
    activeImageNodeId,
    activePromptNodeId,
    activeModelNodeId,
    activeMaskNodeId,
    activeColorNodeId,
    nodeContextMenu,
    isPanning,
    isSelecting,
    isDraggingItem,
    isResizingItem,
    isSpacePressed,
    selectionRect,
    activateImageNode,
    activatePromptNode,
    activateModelNode,
    activateMaskNode,
    activateColorNode,
    isNodeSelected,
    isNodeDragging,
    clearActiveNodes,
    closeNodeContextMenu,
    resetEditorUiState,
    onCanvasContextMenu,
    onCanvasMouseDown,
    onItemMouseDown,
    onItemContextMenu,
    onImageNodeMouseDown,
    onPromptNodeMouseDown,
    onModelNodeMouseDown,
    onMaskNodeMouseDown,
    onColorNodeMouseDown,
    onResizeMouseDown,
    readNodeImageDropPayload,
    handleDrop,
    clearSelectedIds,
    clearSelectedNodeKeys,
    removeSelectedNodeKey,
    removeSelectedId,
    focusSelectedItem,
  }
}
