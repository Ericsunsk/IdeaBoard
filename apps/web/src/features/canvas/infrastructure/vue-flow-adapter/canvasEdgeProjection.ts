import type { Edge } from '@vue-flow/core'

type CanvasSemanticEdge = Omit<Edge, 'animated'>

export function toCanvasSemanticEdge(edge: Edge): CanvasSemanticEdge {
  if (!Object.prototype.hasOwnProperty.call(edge, 'animated')) return edge
  const { animated: _animated, ...semanticEdge } = edge
  return semanticEdge
}

export function toCanvasSemanticEdges(edges: readonly Edge[]): Edge[] {
  let didChange = false
  const semanticEdges = edges.map((edge) => {
    const semanticEdge = toCanvasSemanticEdge(edge)
    if (semanticEdge !== edge) didChange = true
    return semanticEdge as Edge
  })
  return didChange ? semanticEdges : (edges as Edge[])
}

export function projectCanvasRenderEdges(
  semanticEdges: readonly Edge[],
  isAnimated: (edge: Edge) => boolean,
): Edge[] {
  let didChange = false
  const renderEdges = semanticEdges.map((edge) => {
    if (edge.targetHandle !== 'in:image') return edge
    const animated = isAnimated(edge)
    if (edge.animated === animated) return edge
    didChange = true
    return { ...edge, animated }
  })
  return didChange ? renderEdges : (semanticEdges as Edge[])
}
