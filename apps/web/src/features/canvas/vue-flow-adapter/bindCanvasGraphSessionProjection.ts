import type { CanvasGraphNodeId, CanvasGraphSession } from '../graph-runtime'
import type { CanvasGroup } from '../../board/domain'
import type { Node } from '@vue-flow/core'
import type { CanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'
import {
  createCanvasGraphCommitProjectionController,
  type CanvasFlowGraphProjection,
} from './canvasGraphCommitProjection'

/**
 * The only Graph → Flow subscription bridge. It intentionally receives the
 * canonical commit event from the Session, so Flow never needs to infer domain
 * changes from its own arrays. A renderer exception is contained here: graph
 * facts have committed already and the controller schedules a full recovery.
 */
export function bindCanvasGraphSessionProjection(options: {
  session: CanvasGraphSession
  graphAdapter: Pick<
    CanvasVueFlowGraphAdapter,
    'applyIncrementalProjection' | 'replaceFullProjection'
  >
  readCurrent: () => CanvasFlowGraphProjection
  projectFull: () => CanvasFlowGraphProjection
  isCurrentAttachment: () => boolean
  readGroups?: (() => CanvasGroup[]) | undefined
  onGroupsChanged?: ((groups: CanvasGroup[]) => void) | undefined
  projectNode?: ((node: Node) => Node) | undefined
  scheduleMicrotask?: ((task: () => void) => void) | undefined
}) {
  const controller = createCanvasGraphCommitProjectionController({
    projectFull: options.projectFull,
    scheduleMicrotask: options.scheduleMicrotask,
    render: (projection) => {
      if (!options.isCurrentAttachment()) return
      if (projection.kind === 'full') {
        options.graphAdapter.replaceFullProjection(projection.graph)
        return
      }
      options.graphAdapter.applyIncrementalProjection({
        nodes: projection.graph.nodes,
        edges: projection.graph.edges,
        changedNodeIds: projection.changedNodeIds,
        changedEdgeIds: projection.changedEdgeIds,
        nodeMutations: projection.nodeMutations,
        edgeMutations: projection.edgeMutations,
      })
    },
  })

  return options.session.subscribe((event) => {
    if (!options.isCurrentAttachment()) return
    if (event.patch.groups.records.length > 0 || event.patch.groups.order.length > 0) {
      const groups = options.readGroups?.()
      if (groups) options.onGroupsChanged?.(groups)
    }
    try {
      controller.apply({
        current: options.readCurrent(),
        patch: event.patch,
        readNode: (nodeId) => options.session.store.getNode(nodeId as CanvasGraphNodeId),
        projectNode: options.projectNode,
      })
    } catch {
      // Projection failures are deliberately non-fatal. The controller keeps a
      // recovery flag and re-projects from the live session on a microtask.
    }
  })
}
