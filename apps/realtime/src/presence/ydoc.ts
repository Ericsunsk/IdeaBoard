// Y.Doc collaboration shape (ADR-0020, mirrors
// apps/api/internal/boards/ydoc.go): one Y.Doc per board with
// getMap('nodes') + getMap('edges'), both keyed by id.
//
// stripTransient whitelist: id/type/position/data/dimensions/measured/style
// persist and sync; selected/dragging/resizing plus any unknown key is
// dropped before broadcast AND before compaction, so transient UI state never
// reaches the update log or the snapshot.

export const yDocNodesKey = 'nodes'
export const yDocEdgesKey = 'edges'

export const maxYUpdateBytes = 1 << 20
export const maxYCompactEntries = 10_000

export type YEntryInput = Record<string, unknown>

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length > 0
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function isFinitePoint(value: unknown): value is { x: number; y: number } {
  return isObject(value) && isFiniteNumber(value.x) && isFiniteNumber(value.y)
}

/** Strip one Y node entry keyed by id. Returns null to drop the entry. */
export function stripYNode(key: string, entry: unknown): YEntryInput | null {
  if (!isObject(entry)) return null
  if (typeof entry.id !== 'string' || entry.id !== key) return null
  if (!isNonEmptyString(entry.type)) return null
  const out: YEntryInput = { id: entry.id, type: entry.type }
  if (entry.position !== undefined && isFinitePoint(entry.position)) {
    out.position = { x: entry.position.x, y: entry.position.y }
  }
  if (entry.data !== undefined && isObject(entry.data)) out.data = entry.data
  if (entry.dimensions !== undefined && isObject(entry.dimensions)) {
    out.dimensions = entry.dimensions
  }
  if (entry.measured !== undefined && isObject(entry.measured)) {
    out.measured = entry.measured
  }
  if (entry.style !== undefined && isObject(entry.style)) out.style = entry.style
  return out
}

/** Strip one Y edge entry keyed by id. Returns null to drop the entry. */
export function stripYEdge(key: string, entry: unknown): YEntryInput | null {
  if (!isObject(entry)) return null
  if (typeof entry.id !== 'string' || entry.id !== key) return null
  if (!isNonEmptyString(entry.source) || !isNonEmptyString(entry.target)) {
    return null
  }
  const out: YEntryInput = { id: entry.id, source: entry.source, target: entry.target }
  if (entry.type !== undefined && isNonEmptyString(entry.type)) {
    out.type = entry.type
  }
  if (entry.data !== undefined && isObject(entry.data)) out.data = entry.data
  if (entry.style !== undefined && isObject(entry.style)) out.style = entry.style
  return out
}

function stripMap(
  raw: unknown,
  strip: (key: string, entry: unknown) => YEntryInput | null,
): Record<string, YEntryInput> {
  const out: Record<string, YEntryInput> = {}
  if (!isObject(raw)) return out
  for (const key of Object.keys(raw).sort()) {
    if (!key.trim()) continue
    const stripped = strip(key, raw[key])
    if (stripped !== null) out[key] = stripped
  }
  return out
}

export type SanitizedYSnapshot = {
  nodes: Record<string, YEntryInput>
  edges: Record<string, YEntryInput>
}

/** Reduce raw nodes/edges inputs to the sanitized snapshot. Throws on overflow. */
export function sanitizeYSnapshot(nodes: unknown, edges: unknown): SanitizedYSnapshot {
  const sanitized = {
    nodes: stripMap(nodes, stripYNode),
    edges: stripMap(edges, stripYEdge),
  }
  if (
    Object.keys(sanitized.nodes).length + Object.keys(sanitized.edges).length >
    maxYCompactEntries
  ) {
    throw new Error('y snapshot exceeds entry budget')
  }
  return sanitized
}

/** Build the compaction envelope posted to Go (validated there by ValidateV1DocumentStrict). */
export function buildYCompactEnvelope(
  schemaVersion: number,
  snapshot: SanitizedYSnapshot,
  state: Uint8Array,
): string {
  return JSON.stringify({
    schema_version: schemaVersion,
    payload: {
      nodes: snapshot.nodes,
      edges: snapshot.edges,
      y_state: Buffer.from(state).toString('base64'),
    },
  })
}
