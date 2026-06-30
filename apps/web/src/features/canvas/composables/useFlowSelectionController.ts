import type { Ref } from 'vue'
import { useVueFlow, type Elements } from '@vue-flow/core'
import type { DismissTransientCanvasUi } from './useFlowTransientUiDismissal'

type SelectableFlowItem = {
  id: string
  selected?: boolean
}

type UseFlowSelectionControllerOptions<
  NodeT extends SelectableFlowItem,
  EdgeT extends SelectableFlowItem,
> = {
  activeTool: Ref<string>
  fullNodes: Ref<NodeT[]>
  fullEdges: Ref<EdgeT[]>
  dismissTransientCanvasUi: DismissTransientCanvasUi
  _deleteSelectedFlowElements: () => void
}

function clearSelectedFlagFromFlowItems<T extends SelectableFlowItem>(
  items: T[],
  shouldClear: (item: T) => boolean = () => true,
) {
  let nextItems: T[] | null = null
  items.forEach((item, index) => {
    if (!item.selected || !shouldClear(item)) return
    if (!nextItems) nextItems = [...items]
    nextItems[index] = { ...item, selected: false }
  })
  return nextItems
}

function setSelectedFlagOnFlowNode<T extends SelectableFlowItem>(
  nodes: T[],
  nodeId: string,
  selected: boolean,
) {
  const index = nodes.findIndex((node) => node.id === nodeId)
  if (index < 0 || nodes[index]?.selected === selected) return null
  const nextNodes = [...nodes]
  nextNodes[index] = { ...nodes[index], selected }
  return nextNodes
}

export function useFlowSelectionController<
  NodeT extends SelectableFlowItem,
  EdgeT extends SelectableFlowItem,
>(options: UseFlowSelectionControllerOptions<NodeT, EdgeT>) {
  const { getSelectedElements, removeSelectedElements, addSelectedNodes, findNode } = useVueFlow()

  function readSelectedElements(): Elements {
    return [...getSelectedElements.value]
  }

  function clearFullGraphSelection(preserveNodeId?: string): boolean {
    const nextFullNodes = clearSelectedFlagFromFlowItems(
      options.fullNodes.value,
      (node) => node.id !== preserveNodeId,
    )
    const nextFullEdges = clearSelectedFlagFromFlowItems(options.fullEdges.value)
    if (nextFullNodes) options.fullNodes.value = nextFullNodes
    if (nextFullEdges) options.fullEdges.value = nextFullEdges
    return !!nextFullNodes || !!nextFullEdges
  }

  function clearFlowSelection(elements?: Elements): boolean {
    const targetElements = elements ?? readSelectedElements()
    const clearedFullGraphSelection = clearFullGraphSelection()
    if (targetElements.length > 0) {
      removeSelectedElements(targetElements)
    }
    return clearedFullGraphSelection || targetElements.length > 0
  }

  function selectFlowNode(nodeId: string, selectionOptions?: { keepExisting?: boolean }): boolean {
    const targetNode = findNode(nodeId)
    if (!targetNode) return false
    const selectedElements = readSelectedElements()
    const keepExisting = selectionOptions?.keepExisting ?? false
    const isAlreadySelected = selectedElements.some((element) => element.id === nodeId)

    if (!keepExisting) {
      const clearedFullGraphSelection = clearFullGraphSelection(nodeId)
      const selectedElementsToClear = isAlreadySelected
        ? selectedElements.filter((element) => element.id !== nodeId)
        : selectedElements
      if (selectedElementsToClear.length > 0) {
        removeSelectedElements(selectedElementsToClear)
      }
      if (isAlreadySelected) {
        const nextFullNodes = setSelectedFlagOnFlowNode(options.fullNodes.value, nodeId, true)
        if (nextFullNodes) options.fullNodes.value = nextFullNodes
        return clearedFullGraphSelection || selectedElementsToClear.length > 0 || !!nextFullNodes
      }
    }
    if (!isAlreadySelected) {
      addSelectedNodes([targetNode])
    }
    const nextFullNodes = setSelectedFlagOnFlowNode(options.fullNodes.value, nodeId, true)
    if (nextFullNodes) options.fullNodes.value = nextFullNodes
    return true
  }

  function selectFlowNodeForContextMenu(nodeId: string): boolean {
    return selectFlowNode(nodeId, { keepExisting: false })
  }

  function onPaneClick() {
    clearFlowSelection()
    options.dismissTransientCanvasUi({
      closeExpand: true,
      closeBoardHeader: true,
      closeContextMenu: true,
    })
  }

  function onFlowNodeClick() {
    if (options.activeTool.value === 'hand') return
    options.dismissTransientCanvasUi({ closeExpand: true })
  }

  function onFlowNodeMouseDown(...args: [string, MouseEvent]) {
    const nodeId = args[0]
    const event = args[1]
    if (event.button === 0) {
      if (options.activeTool.value !== 'hand') {
        selectFlowNode(nodeId, { keepExisting: event.shiftKey })
      }
      options.dismissTransientCanvasUi({ closeExpand: true })
    }
  }

  function deleteSelectedFlowElements() {
    options.dismissTransientCanvasUi({ closeExpand: true, closeContextMenu: true })
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
