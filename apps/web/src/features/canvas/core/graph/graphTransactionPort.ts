import type { CanvasConnection, CanvasNodeItem } from '../../../board/ports/canvas'
import {
  createChangeSet,
  createCommitPatch,
  hasVisibleChanges,
  normalizeTransactionChanges,
} from './graphPatchAlgebra'
import { createSummary } from './graphHistoryProjection'
import {
  stageAddEdge,
  stageAddNode,
  stageBatchDelete,
  stageCreateGroup,
  stageMoveNodeToGroup,
  stageRemoveEdge,
  stageRemoveNode,
  stageSelectEdge,
  stageSelectNode,
  stageUngroup,
  stageUpdateGroupGeometry,
  stageUpdateNode,
} from './graphMutation'
import type {
  CanvasGraphEdgeId,
  CanvasGraphGroupGeometry,
  CanvasGraphGroupId,
  CanvasGraphNodeId,
  CanvasGraphNodeUpdate,
  CanvasGraphRemoveNodeOptions,
  CanvasGraphTransactionDraft,
  CanvasGraphTransactionHost,
  CanvasGraphTransactionOptions,
  CanvasGraphTransactionReason,
  CanvasGraphTransactionSummary,
} from './graphStoreTypes'

/**
 * Stable transaction port for canvas graph callers.
 *
 * This module owns the `CanvasGraphTransaction` interaction boundary and
 * nothing else: staging delegates to the mutation runtime, change math to
 * the patch algebra, and result building to the history/projection summary.
 * Callers depend only on this port (re-exported through
 * `canvasGraphStore`) and never on the focused modules behind it.
 */
export class CanvasGraphTransaction {
  private readonly store: CanvasGraphTransactionHost
  private readonly reason: CanvasGraphTransactionReason
  private readonly options: Required<CanvasGraphTransactionOptions>
  private readonly draft: CanvasGraphTransactionDraft
  private status: 'open' | 'committed' | 'cancelled' = 'open'

  constructor(
    store: CanvasGraphTransactionHost,
    reason: CanvasGraphTransactionReason,
    options: Required<CanvasGraphTransactionOptions>,
  ) {
    this.store = store
    this.reason = reason
    this.options = options
    this.draft = store._createTransactionDraft()
  }

  addNode(node: CanvasNodeItem): this {
    this.assertOpen()
    stageAddNode(this.draft, node)
    return this
  }

  updateNode(nodeId: CanvasGraphNodeId, update: CanvasGraphNodeUpdate): this {
    this.assertOpen()
    stageUpdateNode(this.draft, nodeId, update)
    return this
  }

  removeNode(nodeId: CanvasGraphNodeId, options: CanvasGraphRemoveNodeOptions = {}): this {
    this.assertOpen()
    stageRemoveNode(this.draft, nodeId, options)
    return this
  }

  selectNode(nodeId: CanvasGraphNodeId, isSelected: boolean): this {
    this.assertOpen()
    stageSelectNode(this.draft, nodeId, isSelected)
    return this
  }

  createGroup(
    nodeIds: CanvasGraphNodeId[],
    geometry: CanvasGraphGroupGeometry,
    groupId?: CanvasGraphGroupId,
  ): this {
    this.assertOpen()
    stageCreateGroup(this.draft, nodeIds, geometry, groupId)
    return this
  }

  updateGroupGeometry(groupId: CanvasGraphGroupId, geometry: CanvasGraphGroupGeometry): this {
    this.assertOpen()
    stageUpdateGroupGeometry(this.draft, groupId, geometry)
    return this
  }

  ungroup(groupId: CanvasGraphGroupId): this {
    this.assertOpen()
    stageUngroup(this.draft, groupId)
    return this
  }

  moveNodeToGroup(nodeId: CanvasGraphNodeId, groupId: CanvasGraphGroupId | null): this {
    this.assertOpen()
    stageMoveNodeToGroup(this.draft, nodeId, groupId)
    return this
  }

  addEdge(connection: CanvasConnection): this {
    this.assertOpen()
    stageAddEdge(this.draft, connection)
    return this
  }

  removeEdge(edgeId: CanvasGraphEdgeId): this {
    this.assertOpen()
    stageRemoveEdge(this.draft, edgeId)
    return this
  }

  selectEdge(edgeId: CanvasGraphEdgeId, isSelected: boolean): this {
    this.assertOpen()
    stageSelectEdge(this.draft, edgeId, isSelected)
    return this
  }

  batchDelete(options: {
    nodeIds?: CanvasGraphNodeId[]
    edgeIds?: CanvasGraphEdgeId[]
    shouldRemoveConnectedEdges?: boolean
  }): this {
    this.assertOpen()
    stageBatchDelete(this.draft, options)
    return this
  }

  commit(): CanvasGraphTransactionSummary {
    this.assertOpen()
    const changes = normalizeTransactionChanges(this.draft)
    if (changes.hasDomainChanges && !this.store._canWriteDomain()) return this.cancel()
    const didChange = hasVisibleChanges(changes)
    const commitPatch = createCommitPatch(this.draft)
    if (didChange) this.store._commitTransaction(this.draft)
    this.status = 'committed'
    const summary = createSummary(
      this.reason,
      this.options,
      this.store._readState(),
      changes,
      this.draft,
      commitPatch,
    )
    if (didChange) this.store._publishCommit(summary)
    return summary
  }

  cancel(): CanvasGraphTransactionSummary {
    this.assertOpen(false)
    this.status = 'cancelled'
    return createSummary(this.reason, this.options, this.draft.base, createChangeSet(), this.draft)
  }

  private assertOpen(shouldRequireCurrentRevision = true) {
    if (this.status !== 'open') {
      throw new Error(`CanvasGraphTransaction is already ${this.status}`)
    }
    if (shouldRequireCurrentRevision) this.store._assertTransactionCurrent(this.draft)
  }
}
