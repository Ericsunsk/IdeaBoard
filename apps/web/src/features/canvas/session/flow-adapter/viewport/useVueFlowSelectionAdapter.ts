import type { Ref } from 'vue'
import { useVueFlow, type EdgeChange, type Elements } from '@vue-flow/core'
import { createCanvasSelectionController } from './canvasSelectionController'

export type DismissTransientCanvasUiOptions = {
  shouldCloseOutpaint?: boolean
  shouldCloseBoardHeader?: boolean
  shouldCloseContextMenu?: boolean
  shouldClearNodePanel?: boolean
  shouldHideStrength?: boolean
}

export type DismissTransientCanvasUi = (options?: DismissTransientCanvasUiOptions) => void

type UseVueFlowSelectionAdapterOptions = {
  activeTool: Ref<string>
  dismissTransientCanvasUi: DismissTransientCanvasUi
  onEdgesChange: (changes: EdgeChange[]) => void
  onSelectionChanged?: (elements: Elements) => void
}

type FlowElement = Elements[number]

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

export function useVueFlowSelectionAdapter(options: UseVueFlowSelectionAdapterOptions) {
  const {
    getSelectedElements,
    removeSelectedElements,
    addSelectedNodes,
    findNode,
    nodesSelectionActive: isNodesSelectionActive,
  } = useVueFlow()
  const selectionController = createCanvasSelectionController({
    readElements: () => [...getSelectedElements.value],
    clearElements: (elements) => removeSelectedElements(elements),
    addNodes: (nodes) => addSelectedNodes(nodes as Parameters<typeof addSelectedNodes>[0]),
    findNode,
  })

  function readSelectedElements(): Elements {
    return [...getSelectedElements.value]
  }

  function clearSelection(elements?: Elements): boolean {
    return selectionController.clear(elements ?? readSelectedElements())
  }

  function readSelectedNodeIds(elements = readSelectedElements()): Set<string> {
    return new Set(elements.filter(isFlowNodeElement).map((element) => element.id))
  }

  function onPaneClick() {
    clearSelection()
    options.onSelectionChanged?.([])
    options.dismissTransientCanvasUi({
      shouldCloseOutpaint: true,
      shouldCloseBoardHeader: true,
      shouldCloseContextMenu: true,
    })
  }

  function onFlowNodeClick() {
    if (options.activeTool.value === 'hand') return
    options.dismissTransientCanvasUi({ shouldCloseOutpaint: true })
    options.onSelectionChanged?.(readSelectedElements())
  }

  function onFlowNodeMouseDown(...args: [string, MouseEvent]) {
    const event = args[1]
    if (event.button === 0) {
      options.dismissTransientCanvasUi({ shouldCloseOutpaint: true })
    }
  }

  function onPointerDownCapture(event: PointerEvent) {
    selectionController.reset()
    if (options.activeTool.value !== 'select') return
    if (event.button !== 0 || event.pointerType === 'touch') return
    if (!isCanvasPanePointerTarget(event.target)) return

    selectionController.beginMarquee(event)
  }

  function onSelectionStart(event: MouseEvent) {
    if (options.activeTool.value !== 'select') return
    selectionController.beginMarquee(event)
    options.dismissTransientCanvasUi({
      shouldCloseOutpaint: true,
      shouldCloseBoardHeader: true,
      shouldCloseContextMenu: true,
    })
  }

  function onSelectionEnd(event: MouseEvent) {
    const selectedElements = readSelectedElements()
    selectionController.endMarquee(event, selectedElements)
    options.onSelectionChanged?.(readSelectedElements())

    isNodesSelectionActive.value = readSelectedNodeIds().size > 1
  }

  function filterMarqueeEdgeChanges(changes: EdgeChange[]): EdgeChange[] {
    if (selectionController.phase !== 'marquee') return changes
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
