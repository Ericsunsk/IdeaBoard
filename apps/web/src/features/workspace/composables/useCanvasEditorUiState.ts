import { ref } from 'vue'

export function useCanvasEditorUiState() {
  const isDuplicateModifierPressed = ref(false)

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

  return {
    isDuplicateModifierPressed,
    nodeContextMenu,
    isPanning,
    isSelecting,
    isDraggingItem,
    draggingNodeKeys,
    isResizingItem,
    isSpacePressed,
    selectionRect,
  }
}
