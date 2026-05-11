import { type Ref } from 'vue'
import type { Edge, Node, NodeMouseEvent } from '@vue-flow/core'

type UseFlowSelectionControllerOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  activeTool: Ref<string>
  activeMessageActionNodeId: Ref<string | null>
  hideFlowStyleStrengthControl: () => void
  closeBoardHeaderEditing: () => void
  closeNodeContextMenu: () => void
  _deleteSelectedFlowElements: () => void
  commitNodes: (nodes: Node[]) => void
  commitEdges: (edges: Edge[]) => void
}

export function useFlowSelectionController(options: UseFlowSelectionControllerOptions) {
  function clearFlowSelection(): boolean {
    let hasSelection = false
    const nextNodes = (options.nodes.value as Array<Node & { selected?: boolean }>).map((node) => {
      if (node.selected) {
        hasSelection = true
        return { ...node, selected: false }
      }
      return node
    })
    const nextEdges = (options.edges.value as Array<Edge & { selected?: boolean }>).map((edge) => {
      if (edge.selected) {
        hasSelection = true
        return { ...edge, selected: false }
      }
      return edge
    })
    if (!hasSelection) return false
    options.commitNodes(nextNodes)
    options.commitEdges(nextEdges)
    return true
  }

  function selectSingleFlowNode(nodeId: string): boolean {
    let hasChanged = false
    const nextNodes = (options.nodes.value as Array<Node & { selected?: boolean }>).map((node) => {
      const shouldBeSelected = node.id === nodeId
      const isSelected = !!node.selected
      if (isSelected === shouldBeSelected) return node
      hasChanged = true
      return { ...node, selected: shouldBeSelected }
    })
    const nextEdges = (options.edges.value as Array<Edge & { selected?: boolean }>).map((edge) => {
      if (!edge.selected) return edge
      hasChanged = true
      return { ...edge, selected: false }
    })
    if (!hasChanged) return false
    options.commitNodes(nextNodes)
    options.commitEdges(nextEdges)
    return true
  }

  function onPaneClick() {
    options.closeBoardHeaderEditing()
    options.closeNodeContextMenu()
    options.activeMessageActionNodeId.value = null
    options.hideFlowStyleStrengthControl()
    clearFlowSelection()
  }

  function onFlowNodeClick(_payload: NodeMouseEvent) {
    if (options.activeTool.value === 'hand') return
    options.activeMessageActionNodeId.value = null
    options.hideFlowStyleStrengthControl()
  }

  function onFlowNodeMouseDown(_nodeId: string, event: MouseEvent) {
    if (event.button === 0) {
      options.activeMessageActionNodeId.value = null
      options.hideFlowStyleStrengthControl()
    }
  }

  function deleteSelectedFlowElements() {
    options.activeMessageActionNodeId.value = null
    options.hideFlowStyleStrengthControl()
    options.closeNodeContextMenu()
    options._deleteSelectedFlowElements()
  }

  return {
    clearFlowSelection,
    selectSingleFlowNode,
    onPaneClick,
    onFlowNodeClick,
    onFlowNodeMouseDown,
    deleteSelectedFlowElements,
  }
}
