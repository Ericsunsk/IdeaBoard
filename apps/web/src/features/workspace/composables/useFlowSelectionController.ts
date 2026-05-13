import { type Ref } from 'vue'
import type { Edge, Elements, Node } from '@vue-flow/core'

type UseFlowSelectionControllerOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  activeTool: Ref<string>
  activeImageActionNodeId: Ref<string | null>
  activeImagePromptNodeId?: Ref<string | null>
  hideFlowStyleStrengthControl: () => void
  closeBoardHeaderEditing: () => void
  closeNodeContextMenu: () => void
  _deleteSelectedFlowElements: () => void
  clearSelectedElements: (elements?: Elements) => void
  selectFlowNodes: (nodes: Node[]) => void
}

export function useFlowSelectionController(options: UseFlowSelectionControllerOptions) {
  function getSelectedElements(): Elements {
    return [
      ...(options.nodes.value as Array<Node & { selected?: boolean }>).filter(
        (node) => !!node.selected,
      ),
      ...(options.edges.value as Array<Edge & { selected?: boolean }>).filter(
        (edge) => !!edge.selected,
      ),
    ]
  }

  function clearFlowSelection(elements?: Elements): boolean {
    const targetElements = elements ?? getSelectedElements()
    if (targetElements.length === 0) return false
    options.clearSelectedElements(targetElements)
    return true
  }

  function selectFlowNodeForContextMenu(nodeId: string): boolean {
    const targetNode = options.nodes.value.find((node) => node.id === nodeId)
    if (!targetNode) return false
    const selectedElements = getSelectedElements()
    const hasChanged = selectedElements.length !== 1 || selectedElements[0]?.id !== nodeId
    if (!hasChanged) return false
    if (selectedElements.length > 0) {
      options.clearSelectedElements(selectedElements)
    }
    options.selectFlowNodes([targetNode])
    return true
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
    const event = args[1]
    if (event.button === 0) {
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
