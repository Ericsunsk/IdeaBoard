import { ref, type Ref } from 'vue'

/**
 * Apply-scope state machine for the Image Tool Session.
 *
 * A retouch apply runs async across render, materialize, and recipe writes
 * while the user may cancel, switch boards, or start over. The scope tracks
 * which apply generation is current so late completions cannot touch a newer
 * scope: every check funnels through `isCurrent()`.
 *
 * Lifetime: `begin` (guarded by `isApplying`) -> `isCurrent` checks ->
 * `release` on settle. `cancel` settles from the session side (exit, commit,
 * dismiss) and `onCleanup` runs node cleanup exactly once per registration.
 */
export type ImageToolApplyScope = {
  readonly scopeId: string
  isCurrent(): boolean
  release(): void
}

export type ImageToolApplyScopeHost = {
  readonly isApplying: Ref<boolean>
  readonly pendingNodeId: Ref<string | null>
  readonly scopeId: Ref<string | null>
  begin(nodeId: string, boardId: string | null | undefined): ImageToolApplyScope | null
  notePendingNode(nodeId: string): void
  clearPendingNode(nodeId: string): void
  onCleanup(cleanup: (() => void) | null): void
  cancel(nodeId?: string | null): void
}

export function createImageToolApplyScopeHost(deps: {
  currentBoardId: () => string | null | undefined
  isNodeAlive: (nodeId: string) => boolean
}): ImageToolApplyScopeHost {
  const isApplying = ref(false)
  const pendingNodeId = ref<string | null>(null)
  const scopeId = ref<string | null>(null)
  let latestApplyId = 0
  let cleanup: (() => void) | null = null

  function runCleanup() {
    cleanup?.()
    cleanup = null
  }

  function settleScope() {
    runCleanup()
    scopeId.value = null
    pendingNodeId.value = null
    isApplying.value = false
  }

  function begin(nodeId: string, boardId: string | null | undefined): ImageToolApplyScope | null {
    if (isApplying.value) return null
    isApplying.value = true
    const applyId = ++latestApplyId
    scopeId.value = nodeId
    const isCurrent = () =>
      applyId === latestApplyId &&
      deps.currentBoardId() === boardId &&
      scopeId.value === nodeId &&
      deps.isNodeAlive(nodeId)
    const release = () => {
      if (applyId !== latestApplyId || scopeId.value !== nodeId) return
      settleScope()
    }
    return { scopeId: nodeId, isCurrent, release }
  }

  function notePendingNode(nodeId: string) {
    pendingNodeId.value = nodeId
  }

  function clearPendingNode(nodeId: string) {
    if (pendingNodeId.value === nodeId) pendingNodeId.value = null
  }

  function onCleanup(next: (() => void) | null) {
    runCleanup()
    cleanup = next
  }

  function cancel(nodeId?: string | null) {
    const target = nodeId ?? scopeId.value
    if (!target) return
    if (scopeId.value === target) settleScope()
  }

  return {
    isApplying,
    pendingNodeId,
    scopeId,
    begin,
    notePendingNode,
    clearPendingNode,
    onCleanup,
    cancel,
  }
}
