import type { Ref } from 'vue'
import { useVueFlow, type EdgeChange, type Elements } from '@vue-flow/core'
import type { DismissTransientCanvasUi } from '../composables/useFlowTransientUiDismissal'

type UseFlowPaneSelectionAdapterOptions = {
  activeTool: Ref<string>
  dismissTransientCanvasUi: DismissTransientCanvasUi
  onEdgesChange: (changes: EdgeChange[]) => void
}

type FlowElement = Elements[number]

type MarqueeSelectionSnapshot = {
  selectedNodeIds: Set<string>
  isToggleMode: boolean
  startX: number
  startY: number
}

const MARQUEE_CLICK_TOLERANCE_PX = 1

function isFlowEdgeElement(element: FlowElement): boolean {
  const candidate = element as { source?: unknown; target?: unknown }
  return typeof candidate.source === 'string' && typeof candidate.target === 'string'
}

function isFlowNodeElement(element: FlowElement): boolean {
  return !isFlowEdgeElement(element)
}

function isCanvasPanePointerTarget(target: EventTarget | null): boolean {
  return target instanceof Element && target.classList.contains('vue-flow__pane')
}

function isPointerMoved(snapshot: MarqueeSelectionSnapshot, event: MouseEvent): boolean {
  return (
    Math.abs(event.clientX - snapshot.startX) > MARQUEE_CLICK_TOLERANCE_PX ||
    Math.abs(event.clientY - snapshot.startY) > MARQUEE_CLICK_TOLERANCE_PX
  )
}

export function useFlowPaneSelectionAdapter(options: UseFlowPaneSelectionAdapterOptions) {
  const {
    getSelectedElements,
    removeSelectedElements,
    addSelectedNodes,
    findNode,
    nodesSelectionActive,
  } = useVueFlow()
  let marqueeSelectionSnapshot: MarqueeSelectionSnapshot | null = null
  let isMarqueeSelectionActive = false

  function readSelectedElements(): Elements {
    return [...getSelectedElements.value]
  }

  function clearSelection(elements?: Elements): boolean {
    const targetElements = elements ?? readSelectedElements()
    if (targetElements.length > 0) {
      removeSelectedElements(targetElements)
    }
    return targetElements.length > 0
  }

  function readSelectedNodeIds(elements = readSelectedElements()): Set<string> {
    return new Set(elements.filter(isFlowNodeElement).map((element) => element.id))
  }

  function onPaneClick() {
    clearSelection()
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

  function onPointerDownCapture(event: PointerEvent) {
    marqueeSelectionSnapshot = null
    if (options.activeTool.value !== 'select') return
    if (event.button !== 0 || event.pointerType === 'touch') return
    if (!isCanvasPanePointerTarget(event.target)) return

    marqueeSelectionSnapshot = {
      selectedNodeIds: readSelectedNodeIds(),
      isToggleMode: event.shiftKey,
      startX: event.clientX,
      startY: event.clientY,
    }
  }

  function onSelectionStart(event: MouseEvent) {
    if (options.activeTool.value !== 'select') return
    isMarqueeSelectionActive = true
    if (!marqueeSelectionSnapshot) {
      marqueeSelectionSnapshot = {
        selectedNodeIds: readSelectedNodeIds(),
        isToggleMode: event.shiftKey,
        startX: event.clientX,
        startY: event.clientY,
      }
    }
    options.dismissTransientCanvasUi({
      closeExpand: true,
      closeBoardHeader: true,
      closeContextMenu: true,
    })
  }

  function reconcileShiftMarqueeSelection(
    snapshot: MarqueeSelectionSnapshot,
    selectedNodes: FlowElement[],
    event: MouseEvent,
  ) {
    const marqueeNodeIds = new Set(selectedNodes.map((node) => node.id))
    const didDragMarquee = isPointerMoved(snapshot, event) || marqueeNodeIds.size > 0
    if (!snapshot.isToggleMode || !didDragMarquee) return

    const nodesToToggleOff = selectedNodes.filter((node) => snapshot.selectedNodeIds.has(node.id))
    if (nodesToToggleOff.length > 0) {
      removeSelectedElements(nodesToToggleOff)
    }

    const nodesToRestore = [...snapshot.selectedNodeIds]
      .filter((nodeId) => !marqueeNodeIds.has(nodeId))
      .map((nodeId) => findNode(nodeId))
      .filter((node): node is NonNullable<typeof node> => !!node)

    if (nodesToRestore.length > 0) {
      addSelectedNodes(nodesToRestore)
    }
  }

  function onSelectionEnd(event: MouseEvent) {
    const snapshot = marqueeSelectionSnapshot
    marqueeSelectionSnapshot = null
    isMarqueeSelectionActive = false

    const selectedElements = readSelectedElements()
    const selectedEdges = selectedElements.filter(isFlowEdgeElement)
    if (selectedEdges.length > 0) {
      removeSelectedElements(selectedEdges)
    }

    if (snapshot) {
      reconcileShiftMarqueeSelection(snapshot, selectedElements.filter(isFlowNodeElement), event)
    }

    nodesSelectionActive.value = readSelectedNodeIds().size > 1
  }

  function filterMarqueeEdgeChanges(changes: EdgeChange[]): EdgeChange[] {
    if (!isMarqueeSelectionActive) return changes
    return changes.filter((change) => change.type !== 'select')
  }

  function onEdgesChange(changes: EdgeChange[]) {
    const filteredChanges = filterMarqueeEdgeChanges(changes)
    if (filteredChanges.length === 0) return
    options.onEdgesChange(filteredChanges)
  }

  return {
    paneSelectionBindings: {
      onPointerDownCapture,
    },
    vueFlowSelectionBindings: {
      onSelectionStart,
      onSelectionEnd,
      onEdgesChange,
    },
    commands: {
      clearSelection,
      onPaneClick,
      onFlowNodeClick,
      onFlowNodeMouseDown,
    },
  }
}
