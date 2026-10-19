import { computed, onScopeDispose, ref, toValue, watch, type MaybeRefOrGetter, type Ref } from 'vue'
import type * as Y from 'yjs'
import type { BoardSnapshot } from '../../../board/ports/canvas'
import { useDocBinding } from './internal/useDocBinding'
import { useMemberActivity } from './internal/useMemberActivity'
import type { PresenceActivity } from './presence/state'

type BoardContext = {
  boardId: MaybeRefOrGetter<string | null | undefined>
  accessState: MaybeRefOrGetter<string | null | undefined>
  canWrite: MaybeRefOrGetter<boolean>
}

type CollaborationTransport = {
  sharedDoc: Ref<Y.Doc | null>
  localClientId: Ref<number | null>
  isDocSynced: Ref<boolean>
}

type TransportContext = {
  activity: Ref<PresenceActivity | null>
  boardId: Ref<string | null>
  accessState: Ref<string | null>
  canWrite: Ref<boolean>
}

export type CanvasCollaborationSession = ReturnType<typeof useCanvasCollaborationSession>

/** Owns the Board residency, transport readiness and canonical graph exchange. */
export function useCanvasCollaborationSession<T extends CollaborationTransport>(options: {
  createTransport: (context: TransportContext) => T
}) {
  const boardId = ref<string | null>(null)
  const accessState = ref<string | null>(null)
  const hasWriteCapability = ref(false)
  const activityState = ref<PresenceActivity | null>(null)
  const transport = options.createTransport({
    boardId,
    accessState,
    canWrite: hasWriteCapability,
    activity: activityState,
  })
  const isEnabled = computed(() => accessState.value === 'workspace' && !!boardId.value)
  // Shared Boards have one persistence path. Until the room is ready,
  // local edits would exist only in memory and disappear on leave/reload.
  const canWrite = computed(
    () =>
      isEnabled.value &&
      hasWriteCapability.value &&
      transport.isDocSynced.value &&
      transport.sharedDoc.value !== null &&
      transport.localClientId.value !== null,
  )
  const activity = useMemberActivity({ isEnabled: canWrite, state: activityState })
  const isReadOnly = computed(() => isEnabled.value && !canWrite.value)
  const binding = useDocBinding({
    boardId,
    doc: transport.sharedDoc,
    deviceId: transport.localClientId,
    canWrite,
    isReady: transport.isDocSynced,
  })
  let graph: { boardId: string; applySnapshot: (snapshot: BoardSnapshot) => void } | null = null
  let stopBoardObservation: (() => void) | undefined

  function projectRemote(remote?: BoardSnapshot | null): void {
    if (!isEnabled.value || !transport.isDocSynced.value || graph?.boardId !== boardId.value) return
    const snapshot = remote === undefined ? binding.readSnapshot() : remote
    if (snapshot) graph?.applySnapshot(snapshot)
  }

  watch(binding.remoteSnapshot, (snapshot) => projectRemote(snapshot), { flush: 'sync' })
  watch(transport.isDocSynced, () => projectRemote(), { flush: 'sync' })
  watch(
    boardId,
    () => {
      graph = null
    },
    { flush: 'sync' },
  )

  function bindBoard(context: BoardContext): void {
    stopBoardObservation?.()
    stopBoardObservation = watch(
      () =>
        [
          toValue(context.boardId),
          toValue(context.accessState),
          toValue(context.canWrite),
        ] as const,
      ([id, access, canWriteBoard]) => {
        if (boardId.value !== (id ?? null)) activity.clear()
        boardId.value = id ?? null
        accessState.value = access ?? null
        hasWriteCapability.value = canWriteBoard
      },
      { immediate: true, flush: 'sync' },
    )
  }

  function attachGraph(applySnapshot: (snapshot: BoardSnapshot) => void): void {
    const id = boardId.value
    if (!id) return
    graph = { boardId: id, applySnapshot }
    // Read the document again: an empty room is a valid authoritative graph,
    // and a room may have completed synchronization before HTTP hydration.
    projectRemote()
  }

  function detachGraph(): void {
    graph = null
  }

  onScopeDispose(() => {
    stopBoardObservation?.()
    graph = null
  })

  return {
    transport,
    isEnabled,
    canWrite,
    isReadOnly,
    bindBoard,
    attachGraph,
    detachGraph,
    activity,
    syncSnapshot: binding.syncSnapshot,
  }
}
