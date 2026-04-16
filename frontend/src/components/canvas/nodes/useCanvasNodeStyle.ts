import { computed } from 'vue'

interface PositionedNodeLike {
  x: number
  y: number
  width: number
}

export function useCanvasNodeStyle(getNode: () => PositionedNodeLike) {
  return computed(() => {
    const node = getNode()
    return {
      left: '50%',
      top: '50%',
      transform: `translate(${node.x}px, ${node.y}px)`,
      width: `${node.width}px`,
    }
  })
}
