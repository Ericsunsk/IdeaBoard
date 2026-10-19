import type { BoardSnapshot } from '../../../../board/ports/canvas'
// Canonical graph commits write changed ids into the shared Y.Doc. Remote
// transactions project back into GraphSession; local origins never echo.

import { DOC_EDGES_MAP, DOC_NODES_MAP, docEntriesToSnapshot, snapshotToDocEntries } from './mapping'

export type DocDeviceId = number | string

export type YMapPort = {
  set: (key: string, value: unknown) => void
  delete: (key: string) => void
  entries: () => Array<[string, unknown]>
  observe: (listener: (events: unknown[], transaction: { origin: unknown }) => void) => void
  unobserve: (listener: (events: unknown[], transaction: { origin: unknown }) => void) => void
}

export type YDocPort = {
  transact: (fn: () => void, origin: DocDeviceId) => void
  getMap: (name: string) => YMapPort
}

type DocBindingMaps = {
  nodes: YMapPort
  edges: YMapPort
}

type DocBindingOptions = {
  doc: YDocPort
  deviceId: DocDeviceId
  canWrite: boolean | (() => boolean)
  onRemoteSnapshot?: ((snapshot: BoardSnapshot) => void) | undefined
}

function readCanWrite(canWrite: boolean | (() => boolean)): boolean {
  return typeof canWrite === 'function' ? canWrite() : canWrite
}

export type DocBinding = {
  syncSnapshot: (
    snapshot: BoardSnapshot,
    nodeIds: readonly string[],
    edgeIds: readonly string[],
  ) => void
  dispose: () => void
}

/** Create the Y.Doc binding over the given doc port. */
export function createDocBinding(options: DocBindingOptions): DocBinding {
  const maps: DocBindingMaps = {
    nodes: options.doc.getMap(DOC_NODES_MAP),
    edges: options.doc.getMap(DOC_EDGES_MAP),
  }
  function syncSnapshot(
    snapshot: BoardSnapshot,
    nodeIds: readonly string[],
    edgeIds: readonly string[],
  ): void {
    if (!readCanWrite(options.canWrite)) return
    const entries = snapshotToDocEntries(snapshot)
    const nextNodes = new Map(entries.nodes)
    const nextEdges = new Map(entries.edges)
    options.doc.transact(() => {
      for (const key of new Set(nodeIds)) {
        const value = nextNodes.get(key)
        if (!value) maps.nodes.delete(key)
        else maps.nodes.set(key, value)
      }
      for (const key of new Set(edgeIds)) {
        const value = nextEdges.get(key)
        if (!value) maps.edges.delete(key)
        else maps.edges.set(key, value)
      }
    }, options.deviceId)
  }

  function onObserve(_events: unknown[], transaction: { origin: unknown }): void {
    if (transaction?.origin === options.deviceId) return
    if (!options.onRemoteSnapshot) return
    const snapshot = readRemoteSnapshot(maps)
    if (snapshot) options.onRemoteSnapshot(snapshot)
  }

  maps.nodes.observe(onObserve)
  maps.edges.observe(onObserve)

  function dispose(): void {
    maps.nodes.unobserve(onObserve)
    maps.edges.unobserve(onObserve)
  }

  return { syncSnapshot, dispose }
}

function readRemoteSnapshot(maps: DocBindingMaps): BoardSnapshot | null {
  return docEntriesToSnapshot(new Map(maps.nodes.entries()), new Map(maps.edges.entries()))
}
