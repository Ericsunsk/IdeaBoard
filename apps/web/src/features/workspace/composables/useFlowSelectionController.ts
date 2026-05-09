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
}

export function useFlowSelectionController(options: UseFlowSelectionControllerOptions) {
  let lastManualNodeSelection: { nodeId: string; timestamp: number } | null = null

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
    options.nodes.value = nextNodes
    options.edges.value = nextEdges
    return true
  }

  function onPaneClick() {
    options.closeBoardHeaderEditing()
    options.closeNodeContextMenu()
    options.activeMessageActionNodeId.value = null
    options.hideFlowStyleStrengthControl()
    clearFlowSelection()
  }

  function onFlowNodeClick(payload: NodeMouseEvent) {
    if (options.activeTool.value === 'hand') return
    options.activeMessageActionNodeId.value = null
    options.hideFlowStyleStrengthControl()
    const targetNodeId = payload.node.id
    const event = payload.event
    if (shouldSkipNodeClickAfterManualSelection(targetNodeId)) return
    applyFlowNodeSelection(targetNodeId, event instanceof MouseEvent ? event : null)
  }

  function applyFlowNodeSelection(targetNodeId: string, event: MouseEvent | null) {
    if (options.activeTool.value === 'hand') return
    const isShiftToggle = !!event?.shiftKey

    if (!isShiftToggle) {
      for (const node of (options.nodes.value as Array<{ id: string; selected?: boolean }>)) {
        const shouldBeSelected = node.id === targetNodeId
        if (node.selected !== shouldBeSelected) {
          node.selected = shouldBeSelected
        }
      }
      for (const edge of (options.edges.value as Array<{ selected?: boolean }>)) {
        if (edge.selected) {
          edge.selected = false
        }
      }
    } else {
      for (const node of (options.nodes.value as Array<{ id: string; selected?: boolean }>)) {
        if (node.id === targetNodeId) {
          node.selected = !node.selected
          break
        }
      }
    }
  }

  function onFlowNodeMouseDown(nodeId: string, event: MouseEvent) {
    if (event.button === 0) {
      options.activeMessageActionNodeId.value = null
      options.hideFlowStyleStrengthControl()
    }
    applyFlowNodeSelection(nodeId, event)
    lastManualNodeSelection = { nodeId, timestamp: Date.now() }
  }

  function shouldSkipNodeClickAfterManualSelection(nodeId: string): boolean {
    if (!lastManualNodeSelection) return false
    const isSameNode = lastManualNodeSelection.nodeId === nodeId
    const isRecent = Date.now() - lastManualNodeSelection.timestamp < 400
    if (!isSameNode || !isRecent) {
      lastManualNodeSelection = null
      return false
    }
    lastManualNodeSelection = null
    return true
  }

  function deleteSelectedFlowElements() {
    options.activeMessageActionNodeId.value = null
    options.hideFlowStyleStrengthControl()
    options.closeNodeContextMenu()
    options._deleteSelectedFlowElements()
  }

  return {
    clearFlowSelection,
    onPaneClick,
    onFlowNodeClick,
    onFlowNodeMouseDown,
    applyFlowNodeSelection,
    shouldSkipNodeClickAfterManualSelection,
    deleteSelectedFlowElements,
  }
}
