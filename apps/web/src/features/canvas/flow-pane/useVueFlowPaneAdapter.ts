import { useVueFlow } from '@vue-flow/core'
import type { CanvasFlowPaneAdapter } from './types'

export function useVueFlowPaneAdapter(): CanvasFlowPaneAdapter {
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
  }
}
