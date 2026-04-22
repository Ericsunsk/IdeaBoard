import type { ViewpointNodeItem } from '../types/canvasNodes'

export function useViewpointNode() {
  function setViewpointSelection(node: ViewpointNodeItem, key: string) {
    node.selectedViewpoint = node.selectedViewpoint === key ? '' : key
  }

  return {
    setViewpointSelection,
  }
}
