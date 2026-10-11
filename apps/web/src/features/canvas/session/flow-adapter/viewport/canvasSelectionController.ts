import type { Elements, Node } from '@vue-flow/core'

type CanvasSelectionPhase = 'idle' | 'marquee'

type SelectionControllerOptions = {
  readElements: () => Elements
  clearElements: (elements: Elements) => void
  addNodes: (nodes: Node[]) => void
  findNode: (nodeId: string) => Node | undefined
}

type MarqueeState = {
  selectedNodeIds: Set<string>
  toggle: boolean
  startX: number
  startY: number
}

const CLICK_TOLERANCE_PX = 1

function isNode(element: Elements[number]): element is Node {
  return !('source' in element && 'target' in element)
}

export function createCanvasSelectionController(options: SelectionControllerOptions) {
  let phase: CanvasSelectionPhase = 'idle'
  let marquee: MarqueeState | null = null

  function readNodes(elements = options.readElements()) {
    return elements.filter(isNode)
  }

  function clear(elements = options.readElements()) {
    marquee = null
    phase = 'idle'
    if (elements.length === 0) return false
    options.clearElements(elements)
    return true
  }

  function beginMarquee(event: Pick<MouseEvent, 'clientX' | 'clientY' | 'shiftKey'>) {
    phase = 'marquee'
    marquee = {
      selectedNodeIds: new Set(readNodes().map((node) => node.id)),
      toggle: event.shiftKey,
      startX: event.clientX,
      startY: event.clientY,
    }
  }

  function endMarquee(
    event: Pick<MouseEvent, 'clientX' | 'clientY'>,
    selectedElements = options.readElements(),
  ) {
    const snapshot = marquee
    marquee = null
    phase = 'idle'
    const selectedNodes = selectedElements.filter(isNode)
    const selectedEdges = selectedElements.filter((element) => !isNode(element))
    if (selectedEdges.length > 0) options.clearElements(selectedEdges)
    if (!snapshot || !snapshot.toggle) return

    const moved =
      Math.abs(event.clientX - snapshot.startX) > CLICK_TOLERANCE_PX ||
      Math.abs(event.clientY - snapshot.startY) > CLICK_TOLERANCE_PX
    if (!moved && selectedNodes.length === 0) return

    const selectedIds = new Set(selectedNodes.map((node) => node.id))
    const nodesToUnselect = selectedNodes.filter((node) => snapshot.selectedNodeIds.has(node.id))
    if (nodesToUnselect.length > 0) options.clearElements(nodesToUnselect)
    const nodesToRestore = [...snapshot.selectedNodeIds]
      .filter((nodeId) => !selectedIds.has(nodeId))
      .map((nodeId) => options.findNode(nodeId))
      .filter((node): node is Node => !!node)
    if (nodesToRestore.length > 0) options.addNodes(nodesToRestore)
  }

  function reset() {
    marquee = null
    phase = 'idle'
  }

  return {
    get phase() {
      return phase
    },
    clear,
    beginMarquee,
    endMarquee,
    reset,
  }
}
