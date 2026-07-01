import type { Ref } from 'vue'
import { useVueFlow, type Elements } from '@vue-flow/core'
import type { DismissTransientCanvasUi } from './useFlowTransientUiDismissal'

type UseFlowSelectionControllerOptions = {
  activeTool: Ref<string>
  dismissTransientCanvasUi: DismissTransientCanvasUi
}

export function useFlowSelectionController(options: UseFlowSelectionControllerOptions) {
  const { getSelectedElements, removeSelectedElements, addSelectedNodes, findNode } = useVueFlow()

  function readSelectedElements(): Elements {
    return [...getSelectedElements.value]
  }

  function clearFlowSelection(elements?: Elements): boolean {
    const targetElements = elements ?? readSelectedElements()
    if (targetElements.length > 0) {
      removeSelectedElements(targetElements)
    }
    return targetElements.length > 0
  }

  function selectFlowNode(nodeId: string, selectionOptions?: { keepExisting?: boolean }): boolean {
    const targetNode = findNode(nodeId)
    if (!targetNode) return false
    const selectedElements = readSelectedElements()
    const keepExisting = selectionOptions?.keepExisting ?? false
    const isAlreadySelected = selectedElements.some((element) => element.id === nodeId)

    if (!keepExisting) {
      const selectedElementsToClear = isAlreadySelected
        ? selectedElements.filter((element) => element.id !== nodeId)
        : selectedElements
      if (selectedElementsToClear.length > 0) {
        removeSelectedElements(selectedElementsToClear)
      }
      if (isAlreadySelected) {
        return true
      }
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
    const event = args[1]
    if (event.button === 0) {
      options.dismissTransientCanvasUi({ closeExpand: true })
    }
  }

  return {
    clearFlowSelection,
    selectFlowNodeForContextMenu,
    onPaneClick,
    onFlowNodeClick,
    onFlowNodeMouseDown,
  }
}
