import { type Ref } from 'vue'
import { useVueFlow, type Elements } from '@vue-flow/core'

type UseFlowSelectionControllerOptions = {
  activeTool: Ref<string>
  activeImageActionNodeId: Ref<string | null>
  activeImagePromptNodeId?: Ref<string | null>
  hideFlowStyleStrengthControl: () => void
  closeBoardHeaderEditing: () => void
  closeNodeContextMenu: () => void
  _deleteSelectedFlowElements: () => void
}

export function useFlowSelectionController(options: UseFlowSelectionControllerOptions) {
  const { getSelectedElements, removeSelectedElements, addSelectedNodes, findNode } = useVueFlow()

  function readSelectedElements(): Elements {
    return [...getSelectedElements.value]
  }

  function clearFlowSelection(elements?: Elements): boolean {
    const targetElements = elements ?? readSelectedElements()
    if (targetElements.length === 0) return false
    removeSelectedElements(targetElements)
    return true
  }

  function selectFlowNode(nodeId: string, options?: { keepExisting?: boolean }): boolean {
    const targetNode = findNode(nodeId)
    if (!targetNode) return false
    const selectedElements = readSelectedElements()
    const keepExisting = options?.keepExisting ?? false
    const isAlreadySelected = selectedElements.some((element) => element.id === nodeId)

    if (!keepExisting && selectedElements.length === 1 && isAlreadySelected) return false
    if (!keepExisting && selectedElements.length > 0) {
      removeSelectedElements(selectedElements)
    }
    if (!isAlreadySelected) {
      addSelectedNodes([targetNode])
    }
    return true
  }

  function selectFlowNodeForContextMenu(nodeId: string): boolean {
    return selectFlowNode(nodeId, { keepExisting: false })
  }

  function onPaneClick() {
    options.closeBoardHeaderEditing()
    options.closeNodeContextMenu()
    options.activeImageActionNodeId.value = null
    if (options.activeImagePromptNodeId) options.activeImagePromptNodeId.value = null
    options.hideFlowStyleStrengthControl()
  }

  function onFlowNodeClick() {
    if (options.activeTool.value === 'hand') return
    options.activeImageActionNodeId.value = null
    if (options.activeImagePromptNodeId) options.activeImagePromptNodeId.value = null
    options.hideFlowStyleStrengthControl()
  }

  function onFlowNodeMouseDown(...args: [string, MouseEvent]) {
    const nodeId = args[0]
    const event = args[1]
    if (event.button === 0) {
      if (options.activeTool.value !== 'hand') {
        selectFlowNode(nodeId, { keepExisting: event.shiftKey })
      }
      options.activeImageActionNodeId.value = null
      if (options.activeImagePromptNodeId) options.activeImagePromptNodeId.value = null
      options.hideFlowStyleStrengthControl()
    }
  }

  function deleteSelectedFlowElements() {
    options.activeImageActionNodeId.value = null
    if (options.activeImagePromptNodeId) options.activeImagePromptNodeId.value = null
    options.hideFlowStyleStrengthControl()
    options.closeNodeContextMenu()
    options._deleteSelectedFlowElements()
  }

  return {
    clearFlowSelection,
    selectFlowNodeForContextMenu,
    onPaneClick,
    onFlowNodeClick,
    onFlowNodeMouseDown,
    deleteSelectedFlowElements,
  }
}
