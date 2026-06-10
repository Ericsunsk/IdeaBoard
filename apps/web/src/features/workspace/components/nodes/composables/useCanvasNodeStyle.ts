import { computed } from 'vue'

interface SizedNodeLike {
  width: number
}

export function useCanvasNodeStyle(getNode: () => SizedNodeLike) {
  return computed(() => {
    const node = getNode()
    return {
      width: `${node.width}px`,
    }
  })
}
