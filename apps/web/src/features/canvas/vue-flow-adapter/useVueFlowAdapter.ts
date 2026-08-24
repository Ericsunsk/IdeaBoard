import { useVueFlow } from '@vue-flow/core'
import type { CanvasVueFlowAdapter } from './types'
import { createCanvasVueFlowRendererMetricsPort } from './canvasRendererMetrics'

export function useVueFlowAdapter(): CanvasVueFlowAdapter {
  const {
    screenToFlowCoordinate,
    updateNodeInternals,
    zoomIn,
    zoomOut,
    viewport,
    panBy,
    getViewport,
    setViewport,
    setCenter,
    getSelectedElements,
    removeSelectedElements,
    addSelectedNodes,
    findNode,
    setNodes,
    setEdges,
    addNodes,
    addEdges,
    removeNodes,
    removeEdges,
    updateNode,
    applyNodeChanges,
    applyEdgeChanges,
  } = useVueFlow()

  function selectOnlyNode(nodeId: string): boolean {
    const node = findNode(nodeId)
    if (!node) return false

    const selectedElements = [...getSelectedElements.value]
    const isOnlySelected = selectedElements.length === 1 && selectedElements[0]?.id === nodeId
    if (isOnlySelected) return true

    if (selectedElements.length > 0) removeSelectedElements(selectedElements)
    addSelectedNodes([node])
    return true
  }

  return {
    viewport,
    getViewport,
    setViewport: (nextViewport, options) => setViewport(nextViewport, options),
    setCenter: (x, y, options) => setCenter(x, y, options),
    zoomIn,
    zoomOut,
    panBy,
    updateNodeInternals,
    screenToFlowCoordinate,
    getSelectedElements,
    removeSelectedElements,
    selectOnlyNode,
    vueFlowGraphActions: {
      setNodes,
      setEdges,
      addNodes,
      addEdges,
      removeNodes,
      removeEdges,
      updateNode,
      applyNodeChanges,
      applyEdgeChanges,
    },
    rendererMetrics: createCanvasVueFlowRendererMetricsPort(findNode),
  }
}
