import { type Ref } from 'vue'
import { useVueFlow, type Elements } from '@vue-flow/core'
import type { DismissTransientWorkspaceUi } from './useFlowTransientUiDismissal'

type UseFlowSelectionControllerOptions = {
  activeTool: Ref<string>
  dismissTransientWorkspaceUi: DismissTransientWorkspaceUi
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
    options.dismissTransientWorkspaceUi({
      closeExpand: true,
      closeBoardHeader: true,
      closeContextMenu: true,
    })
  }

  function onFlowNodeClick() {
    if (options.activeTool.value === 'hand') return
    options.dismissTransientWorkspaceUi({ closeExpand: true })
  }

  function onFlowNodeMouseDown(...args: [string, MouseEvent]) {
    const nodeId = args[0]
    const event = args[1]
    if (event.button === 0) {
      if (options.activeTool.value !== 'hand') {
        selectFlowNode(nodeId, { keepExisting: event.shiftKey })
      }
      options.dismissTransientWorkspaceUi({ closeExpand: true })
    }
  }

  function deleteSelectedFlowElements() {
    options.dismissTransientWorkspaceUi({ closeExpand: true, closeContextMenu: true })
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
