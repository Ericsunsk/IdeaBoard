import {
  assertLatestBoardSnapshot,
  BOARD_STATE_SCHEMA_VERSION,
  type BoardSnapshot,
} from '../../../../board/ports/canvas'

// Snapshot <-> Y.Doc mapping (Board schema v4 strict).
//
// Frozen contract: `Y.Doc` carries exactly two top-level maps,
// `getMap('nodes')` + `getMap('edges')` (standard yjs, no custom types).
// This module is yjs-free pure data mapping so specs run without sockets:
// `snapshotToDocEntries` flattens a `BoardSnapshot` into map entries ready
// for `Y.Map.set`, and `docEntriesToSnapshot` rebuilds strictly.
//
// - `schemaVersion` must equal 4, otherwise the conversion fails closed
//   (throw on the way in, null on the way out — never a partial document).
// - Groups ride the `nodes` map as `group-<n>` entries (no new top-level
//   type); connections ride the `edges` map as `connection-<n>` entries.
// - `mediaRefs` are derived on save (Board media-ref extraction) and are
//   never synced: they are omitted here and recomputed by the save path.
//
/** Top-level Y.Doc map names (frozen: standard yjs, no custom types). */
export const DOC_NODES_MAP = 'nodes'
export const DOC_EDGES_MAP = 'edges'

/** Transient Vue Flow UI state stripped before any CRDT write. */
const TRANSIENT_NODE_KEYS = ['selected', 'dragging', 'resizing'] as const

type DocNodeKind = 'image' | 'prompt' | 'fusion' | 'group'

type DocNodeEntry = {
  id: string
  type: DocNodeKind
  position: { x: number; y: number }
  data: Record<string, unknown>
}

type DocEdgeEntry = Record<string, unknown>

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function toFiniteNumber(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

/** Strip transient interaction fields (`selected`/`dragging`/`resizing`). */
function stripTransient<T>(node: T): T {
  if (!isRecord(node)) return node
  let didStrip = false
  for (const key of TRANSIENT_NODE_KEYS) {
    if (Object.prototype.hasOwnProperty.call(node, key)) didStrip = true
  }
  if (!didStrip) return node
  const cleaned = { ...node }
  for (const key of TRANSIENT_NODE_KEYS) delete cleaned[key]
  return cleaned as T
}

function nodeKey(kind: DocNodeKind, id: string | number): string {
  if (kind === 'group') return String(id).startsWith('group-') ? String(id) : `group-${String(id)}`
  return `${kind}-${String(id)}`
}

function readNodeId(node: Record<string, unknown>): string | null {
  const id = node.id
  if (typeof id === 'string' && id.trim()) return id.trim()
  if (typeof id === 'number' && Number.isFinite(id)) return String(id)
  return null
}

function assertSnapshotVersion(snapshot: BoardSnapshot): void {
  if (snapshot.schemaVersion !== BOARD_STATE_SCHEMA_VERSION) {
    throw new Error(`Unsupported snapshot schemaVersion ${String(snapshot.schemaVersion)}`)
  }
}

function toStrippedRecord(node: unknown): Record<string, unknown> | null {
  if (!isRecord(node)) return null
  return stripTransient(node)
}

export type DocEntries = {
  nodes: Array<[string, DocNodeEntry]>
  edges: Array<[string, DocEdgeEntry]>
}

function toNodeEntries(
  items: readonly unknown[] | undefined,
  kind: DocNodeKind,
): Array<[string, DocNodeEntry]> {
  const entries: Array<[string, DocNodeEntry]> = []
  for (const item of items ?? []) {
    const cleaned = toStrippedRecord(item)
    if (!cleaned) continue
    const id = readNodeId(cleaned)
    if (!id) continue
    const key = nodeKey(kind, id)
    entries.push([
      key,
      {
        id: key,
        type: kind,
        position: { x: toFiniteNumber(cleaned.x) ?? 0, y: toFiniteNumber(cleaned.y) ?? 0 },
        data: cleaned,
      },
    ])
  }
  return entries
}

/** Flatten a v4 snapshot into `nodes`/`edges` map entries (throws on version). */
export function snapshotToDocEntries(snapshot: BoardSnapshot): DocEntries {
  assertSnapshotVersion(snapshot)
  const nodes: Array<[string, DocNodeEntry]> = [
    ...toNodeEntries(snapshot.imageNodes, 'image'),
    ...toNodeEntries(snapshot.promptNodes, 'prompt'),
    ...toNodeEntries(snapshot.fusionNodes, 'fusion'),
    ...toNodeEntries(snapshot.groups, 'group'),
  ]
  const edges: Array<[string, DocEdgeEntry]> = []
  for (const connection of snapshot.connections ?? []) {
    const cleaned = toStrippedRecord(connection)
    if (!cleaned) continue
    const id = readNodeId(cleaned)
    if (!id) continue
    const key = `connection-${id}`
    edges.push([
      key,
      {
        id: key,
        source: `${String(cleaned.fromNodeType)}-${String(cleaned.fromNodeId)}`,
        target: `${String(cleaned.toNodeType)}-${String(cleaned.toNodeId)}`,
        data: cleaned,
      },
    ])
  }
  return { nodes, edges }
}

function parseDocNodeEntry(
  key: string,
  raw: unknown,
): { kind: DocNodeKind; node: Record<string, unknown> } | null {
  if (!isRecord(raw)) return null
  const kind = raw.type
  if (kind !== 'image' && kind !== 'prompt' && kind !== 'fusion' && kind !== 'group') return null
  if (raw.id !== key || !isRecord(raw.data)) return null
  const originalId = raw.data.id as string | number
  return {
    kind,
    node: {
      ...raw.data,
      id: originalId,
      x: raw.position && isRecord(raw.position) ? raw.position.x : 0,
      y: raw.position && isRecord(raw.position) ? raw.position.y : 0,
    },
  }
}

function parseDocEdgeEntry(raw: unknown): DocEdgeEntry | null {
  if (!isRecord(raw)) return null
  return raw
}

function sortByNumericId(left: Record<string, unknown>, right: Record<string, unknown>): number {
  const leftId = toFiniteNumber(left.id) ?? Number.MAX_SAFE_INTEGER
  const rightId = toFiniteNumber(right.id) ?? Number.MAX_SAFE_INTEGER
  return leftId - rightId
}

function listEntries(
  entries: ReadonlyMap<string, unknown> | Record<string, unknown>,
): Array<[string, unknown]> {
  return entries instanceof Map ? [...entries.entries()] : Object.entries(entries)
}

/**
 * Rebuild a v4 snapshot from `nodes`/`edges` entries. Returns null for any
 * malformed frame (fail-closed, never a partial document). `mediaRefs` are
 * intentionally omitted (derived by the save path, never synced).
 */
export function docEntriesToSnapshot(
  nodeEntries: ReadonlyMap<string, unknown> | Record<string, unknown>,
  edgeEntries: ReadonlyMap<string, unknown> | Record<string, unknown>,
): BoardSnapshot | null {
  try {
    const nodeList = listEntries(nodeEntries)
    const edgeList = listEntries(edgeEntries)
    const imageNodes: Record<string, unknown>[] = []
    const promptNodes: Record<string, unknown>[] = []
    const fusionNodes: Record<string, unknown>[] = []
    const groups: Record<string, unknown>[] = []
    const connections: Record<string, unknown>[] = []
    for (const [key, raw] of nodeList) {
      const entry = parseDocNodeEntry(key, raw)
      if (!entry) return null
      if (readNodeId(entry.node) === null) return null
      if (entry.kind === 'image') imageNodes.push(entry.node)
      else if (entry.kind === 'prompt') promptNodes.push(entry.node)
      else if (entry.kind === 'fusion') fusionNodes.push(entry.node)
      else groups.push(entry.node)
    }
    for (const [, raw] of edgeList) {
      const entry = parseDocEdgeEntry(raw)
      if (!entry) return null
      if (readNodeId(entry) === null) return null
      if (!isRecord(entry.data)) return null
      connections.push(entry.data)
    }
    imageNodes.sort(sortByNumericId)
    promptNodes.sort(sortByNumericId)
    fusionNodes.sort(sortByNumericId)
    connections.sort(sortByNumericId)
    return assertLatestBoardSnapshot(
      {
        schemaVersion: BOARD_STATE_SCHEMA_VERSION,
        imageNodes,
        promptNodes,
        fusionNodes,
        connections,
        groups,
      },
      'collaboration',
    )
  } catch {
    return null
  }
}
