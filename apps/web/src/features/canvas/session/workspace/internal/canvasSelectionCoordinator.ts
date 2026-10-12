import {
  isCanvasGraphNodeId,
  type CanvasGraphEdgeId,
  type CanvasGraphNodeId,
} from '../../../core/graph'
import { parseFlowNodeId } from '../../../session/flow-adapter'
import type { CanvasRendererSelectableElement } from '../model.ts'

type Selectable = { id: string; selected?: boolean }

export type CanvasSelectionStore = {
  getNodeIds: () => readonly CanvasGraphNodeId[]
  getEdgeIds: () => readonly CanvasGraphEdgeId[]
}

export type CanvasSelectionTransaction = {
  selectNode: (nodeId: CanvasGraphNodeId, selected: boolean) => unknown
  selectEdge: (edgeId: CanvasGraphEdgeId, selected: boolean) => unknown
}

function collectSelectedIds(...groups: Array<readonly Selectable[]>): Set<string> {
  const selectedIds = new Set<string>()
  for (const group of groups) {
    for (const item of group) {
      if (item.selected === true) selectedIds.add(item.id)
    }
  }
  return selectedIds
}

function preserveSelectedItems<T extends Selectable>(items: T[], selectedIds: Set<string>): T[] {
  let nextItems: T[] | null = null
  items.forEach((item, index) => {
    const shouldSelect = selectedIds.has(item.id)
    if (item.selected === shouldSelect) return
    nextItems ??= [...items]
    nextItems[index] = { ...item, selected: shouldSelect }
  })
  return nextItems ?? items
}

/** Keeps Vue Flow's transient selection when projecting a canonical graph. */
export function preserveCanvasRendererSelection<N extends Selectable, E extends Selectable>(
  current: { nodes: readonly N[]; edges: readonly E[] },
  next: { nodes: N[]; edges: E[] },
) {
  const selectedNodeIds = collectSelectedIds(current.nodes)
  const selectedEdgeIds = collectSelectedIds(current.edges)
  return {
    nodes: preserveSelectedItems(next.nodes, selectedNodeIds),
    edges: preserveSelectedItems(next.edges, selectedEdgeIds),
  }
}

/** Converts renderer ids to canonical graph ids and drops non-canvas elements. */
export function resolveCanvasRendererSelection(
  elements: readonly CanvasRendererSelectableElement[],
) {
  const selectedNodeIds = new Set<string>()
  const selectedEdgeIds = new Set<string>()
  for (const element of elements) {
    if (!element.selected) continue
    if ('source' in element && 'target' in element) {
      if (/^connection-\d+$/.test(element.id)) selectedEdgeIds.add(element.id)
      continue
    }
    const parsed = parseFlowNodeId(element.id)
    if (!parsed) continue
    const id = `${parsed.nodeType}-${parsed.id}`
    if (isCanvasGraphNodeId(id)) selectedNodeIds.add(id)
  }
  return { selectedNodeIds, selectedEdgeIds }
}

export function stageCanvasRendererSelection(
  store: CanvasSelectionStore,
  transaction: CanvasSelectionTransaction,
  elements: readonly CanvasRendererSelectableElement[],
) {
  const { selectedNodeIds, selectedEdgeIds } = resolveCanvasRendererSelection(elements)
  store
    .getNodeIds()
    .forEach((nodeId) => transaction.selectNode(nodeId, selectedNodeIds.has(nodeId)))
  store
    .getEdgeIds()
    .forEach((edgeId) => transaction.selectEdge(edgeId, selectedEdgeIds.has(edgeId)))
}
