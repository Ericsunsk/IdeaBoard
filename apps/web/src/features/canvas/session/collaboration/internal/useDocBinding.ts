import type { BoardSnapshot } from '../../../../board/ports/canvas'
// Live doc-binding bridge (shared Y.Doc <-> CRDT binding).
//
// Attaches `createDocBinding` to the presence room's shared doc when both
// the doc and the local device id are known. The collaboration session
// mounts the `nodes`/`edges` shared maps on the same doc for workspace
// boards. The binding observes remote `nodes`/`edges` maps into the canonical graph projection. Local writes are
// capability guarded and origin tagged; changed ids wait through sync and
// are cleared when the board changes.

import { onScopeDispose, shallowRef, toValue, watch, type MaybeRefOrGetter } from 'vue'
import type * as Y from 'yjs'
import {
  createDocBinding,
  type DocBinding,
  type DocDeviceId,
  type YDocPort,
  type YMapPort,
} from './binding'
import { docEntriesToSnapshot } from './mapping'

function adaptYMap(map: Y.Map<unknown>): YMapPort {
  type YMapListener = Parameters<Y.Map<unknown>['observe']>[0]
  return {
    set: (key, value) => {
      map.set(key, value)
    },
    delete: (key) => {
      map.delete(key)
    },
    entries: () => [...map.entries()],
    observe: (listener) => {
      map.observe(listener as unknown as YMapListener)
    },
    unobserve: (listener) => {
      map.unobserve(listener as unknown as YMapListener)
    },
  }
}

function adaptYDoc(doc: Y.Doc): YDocPort {
  return {
    transact: (fn, origin: DocDeviceId) => {
      doc.transact(fn, origin)
    },
    getMap: (name) => adaptYMap(doc.getMap(name)),
  }
}

type UseDocBindingOptions = {
  boardId: MaybeRefOrGetter<string | null | undefined>
  doc: MaybeRefOrGetter<Y.Doc | null | undefined>
  deviceId: MaybeRefOrGetter<number | null>
  canWrite: MaybeRefOrGetter<boolean>
  isReady: MaybeRefOrGetter<boolean>
}

export function useDocBinding(options: UseDocBindingOptions) {
  const remoteSnapshot = shallowRef<BoardSnapshot | null>(null)
  let binding: DocBinding | null = null
  const pendingNodes = new Map<string, BoardSnapshot>()
  const pendingEdges = new Map<string, BoardSnapshot>()

  function clearPending(): void {
    pendingNodes.clear()
    pendingEdges.clear()
  }

  function flushPending(): void {
    if (!binding || !toValue(options.isReady) || !toValue(options.canWrite)) return
    for (const [id, snapshot] of pendingNodes) binding.syncSnapshot(snapshot, [id], [])
    for (const [id, snapshot] of pendingEdges) binding.syncSnapshot(snapshot, [], [id])
    clearPending()
  }

  watch(
    () => toValue(options.boardId),
    () => {
      clearPending()
      teardown()
    },
    { flush: 'sync' },
  )
  watch(
    () => toValue(options.canWrite),
    (canWrite) => {
      if (!canWrite) clearPending()
    },
    { flush: 'sync' },
  )

  function teardown(): void {
    binding?.dispose()
    binding = null
    remoteSnapshot.value = null
  }

  function attach(doc: Y.Doc, deviceId: number): void {
    teardown()
    binding = createDocBinding({
      doc: adaptYDoc(doc),
      deviceId,
      canWrite: () => toValue(options.canWrite) && toValue(options.isReady),
      onRemoteSnapshot: (snapshot) => {
        remoteSnapshot.value = snapshot
      },
    })
    flushPending()
    remoteSnapshot.value = docEntriesToSnapshot(
      new Map(doc.getMap('nodes')),
      new Map(doc.getMap('edges')),
    )
  }

  watch(
    () => [toValue(options.doc), toValue(options.deviceId)] as const,
    ([doc, deviceId]) => {
      if (doc && deviceId !== null && Number.isInteger(deviceId)) attach(doc, deviceId)
      else teardown()
    },
    { immediate: true },
  )
  watch(() => toValue(options.isReady), flushPending, { flush: 'sync' })

  function syncSnapshot(
    snapshot: BoardSnapshot,
    nodeIds: readonly string[],
    edgeIds: readonly string[],
  ): void {
    if (!toValue(options.canWrite) || !toValue(options.boardId)) return
    if (binding && toValue(options.isReady)) {
      binding.syncSnapshot(snapshot, nodeIds, edgeIds)
      return
    }
    for (const id of nodeIds) pendingNodes.set(id, snapshot)
    for (const id of edgeIds) pendingEdges.set(id, snapshot)
  }

  function readSnapshot(): BoardSnapshot | null {
    if (!binding) return null
    const doc = toValue(options.doc)
    if (!doc) return null
    return docEntriesToSnapshot(new Map(doc.getMap('nodes')), new Map(doc.getMap('edges')))
  }

  onScopeDispose(teardown)

  return {
    remoteSnapshot,
    syncSnapshot,
    readSnapshot,
  }
}
