import { getCurrentScope, onScopeDispose } from 'vue'
import type {
  DismissTransientCanvasUi,
  FlowSnapshotParamPatch,
} from '../../../session/flow-adapter'

type UseFlowCanvasNodeHandlersOptions = {
  dismissTransientCanvasUi: DismissTransientCanvasUi
  discardFlowImageToolNodeEdits: (nodeId: string) => void
  deleteNode: (nodeId: string) => void
  updatePromptText: (nodeId: string, value: string) => void
  updateNodeParams: (nodeId: string, payload: FlowSnapshotParamPatch) => void
  resetImage: (nodeId: string) => void
  markFlowExecutionNodeDirty: (nodeId: string, options?: { shouldIncludeSelf?: boolean }) => void
  /** Trailing delay for text-only dirty marks; injectable for specs. */
  textDirtyDelayMs?: number | undefined
}

const DEFAULT_TEXT_DIRTY_DELAY_MS = 300

export function useFlowCanvasNodeHandlers(options: UseFlowCanvasNodeHandlersOptions) {
  const textDirtyDelayMs = Math.max(0, options.textDirtyDelayMs ?? DEFAULT_TEXT_DIRTY_DELAY_MS)
  // Prompt text arrives per keystroke. The store commit below stays
  // synchronous so the controlled textarea keeps reflecting each character,
  // but the downstream staleness sweep rebuilds edge/node maps and opens its
  // own transaction + autosave bump — defer it to a trailing edge so a typing
  // burst pays that cost once.
  const pendingTextDirty = new Map<string, ReturnType<typeof setTimeout>>()

  function flushTextDirty(nodeId: string) {
    const timer = pendingTextDirty.get(nodeId)
    if (timer === undefined) return
    pendingTextDirty.delete(nodeId)
    clearTimeout(timer)
    // Best effort: the board may have switched while the trailing call was
    // pending, in which case the store is detached and there is nothing stale
    // left to mark on this board.
    try {
      options.markFlowExecutionNodeDirty(nodeId)
    } catch (error) {
      console.error('[Canvas] Failed to mark execution nodes stale after text edit', error)
    }
  }

  function scheduleTextDirty(nodeId: string) {
    const timer = pendingTextDirty.get(nodeId)
    if (timer !== undefined) clearTimeout(timer)
    pendingTextDirty.set(
      nodeId,
      setTimeout(() => flushTextDirty(nodeId), textDirtyDelayMs),
    )
  }

  if (getCurrentScope()) {
    onScopeDispose(() => {
      for (const nodeId of pendingTextDirty.keys()) {
        const timer = pendingTextDirty.get(nodeId)
        pendingTextDirty.delete(nodeId)
        if (timer !== undefined) clearTimeout(timer)
      }
    })
  }

  function onFlowDeleteNode(nodeId: string) {
    options.dismissTransientCanvasUi({ shouldCloseContextMenu: true })
    options.discardFlowImageToolNodeEdits(nodeId)
    options.deleteNode(nodeId)
  }

  function onFlowPromptTextUpdate(nodeId: string, value: string) {
    options.updatePromptText(nodeId, value)
    scheduleTextDirty(nodeId)
  }

  function onFlowNodeParamUpdate(nodeId: string, payload: FlowSnapshotParamPatch) {
    options.updateNodeParams(nodeId, payload)
    options.markFlowExecutionNodeDirty(nodeId, { shouldIncludeSelf: true })
  }

  function onFlowImageReset(nodeId: string) {
    options.discardFlowImageToolNodeEdits(nodeId)
    options.resetImage(nodeId)
  }

  return {
    onFlowDeleteNode,
    onFlowPromptTextUpdate,
    onFlowNodeParamUpdate,
    onFlowImageReset,
  }
}
