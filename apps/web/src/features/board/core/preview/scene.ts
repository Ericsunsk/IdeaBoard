import { isRecord } from '../../../../core/utils/guards'

// Strict consumer for the canonical Board Preview Scene wire contract in
// apps/api/internal/boards/previewscene/contract.md. The server owns scene
// extraction and budgets; the Gallery reads camelCase domain data only.
const PREVIEW_SCENE_VERSION = 1

const SCENE_WIRE_KEYS = [
  'version',
  'content_version',
  'bounds',
  'extent',
  'nodes',
  'connections',
  'groups',
  'budgets',
  'omitted',
  'truncated',
]
const SCENE_NODE_KEYS = [
  'id',
  'kind',
  'x',
  'y',
  'width',
  'height',
  'aspect',
  'media',
  'blend',
  'prompt',
]
const SCENE_MEDIA_KEYS = ['asset', 'asset_numeric', 'tier', 'role', 'index']
const SCENE_POINT_KEYS = ['x', 'y']
const SCENE_PROMPT_KEYS = ['aspect_ratio', 'has_inputs']
const SCENE_CONNECTION_KEYS = ['key', 'from', 'to', 'from_port', 'to_port', 'start', 'end']
const SCENE_GROUP_KEYS = ['id', 'members', 'x', 'y', 'width', 'height', 'is_empty']
const SCENE_BOUNDS_KEYS = ['min_x', 'min_y', 'max_x', 'max_y', 'width', 'height']
const SCENE_EXTENT_KEYS = ['width', 'height']
const SCENE_BUDGET_KEYS = ['max_nodes', 'max_connections', 'max_media_refs', 'max_scene_bytes']
const SCENE_OMITTED_KEYS = ['nodes', 'connections', 'media_refs', 'groups', 'group_members']

type PreviewScenePoint = { x: number; y: number }

type PreviewSceneBounds = {
  minX: number
  minY: number
  maxX: number
  maxY: number
  width: number
  height: number
}

type PreviewSceneExtent = { width: number; height: number }

export type PreviewSceneMediaRef = {
  asset: string
  assetNumeric: number | null
  tier: string
  role: string
  index: number
}

type PreviewScenePromptShape = { aspectRatio: string; hasInputs: boolean }

export type PreviewSceneNode = {
  id: string
  kind: 'image' | 'prompt' | 'fusion'
  x: number
  y: number
  width: number
  height: number
  aspect: number
  media: PreviewSceneMediaRef[]
  blend?: PreviewScenePoint | undefined
  prompt?: PreviewScenePromptShape | undefined
}

export type PreviewSceneConnection = {
  key: string
  from: string
  to: string
  fromPort: string
  toPort: string
  start: PreviewScenePoint
  end: PreviewScenePoint
}

type PreviewSceneGroup = {
  id: string
  members: string[]
  x: number
  y: number
  width: number
  height: number
  isEmpty: boolean
}

// Null budget limits indicate that no server limit was applied.
type PreviewSceneBudgets = {
  maxNodes: number | null
  maxConnections: number | null
  maxMediaRefs: number | null
  maxSceneBytes: number | null
}

type PreviewSceneOmitted = {
  nodes: number
  connections: number
  mediaRefs: number
  groups: number
  groupMembers: number
}

/** @externalContract Board Preview Scene wire v1, shared with the Go extractor. */
export type PreviewScene = {
  version: number
  contentVersion: string
  bounds: PreviewSceneBounds | null
  extent: PreviewSceneExtent
  nodes: PreviewSceneNode[]
  connections: PreviewSceneConnection[]
  groups: PreviewSceneGroup[]
  budgets: PreviewSceneBudgets
  omitted: PreviewSceneOmitted
  truncated: boolean
}

function isFiniteList(values: ReadonlyArray<unknown>): boolean {
  return values.every((value) => typeof value === 'number' && Number.isFinite(value))
}

function hasOnlyKeys(value: Record<string, unknown>, keys: ReadonlyArray<string>): boolean {
  return Object.keys(value).every((key) => keys.includes(key))
}

function parseAssetNumeric(value: unknown): number | null | undefined {
  if (value === null) return null
  if (typeof value === 'number' && Number.isInteger(value) && value > 0) return value
  return undefined
}

function parseSceneMediaRef(value: unknown): PreviewSceneMediaRef | null {
  if (!isRecord(value) || !hasOnlyKeys(value, SCENE_MEDIA_KEYS)) return null
  const asset = typeof value.asset === 'string' ? value.asset : null
  const assetNumeric = parseAssetNumeric(value.asset_numeric)
  if (asset === null || assetNumeric === undefined) return null
  if (!asset && assetNumeric === null) return null
  if (typeof value.tier !== 'string' || !value.tier) return null
  if (typeof value.role !== 'string' || !value.role) return null
  if (typeof value.index !== 'number' || !Number.isInteger(value.index)) return null
  return { asset, assetNumeric, tier: value.tier, role: value.role, index: value.index }
}

function parseSceneMediaList(value: unknown): PreviewSceneMediaRef[] | null {
  if (!Array.isArray(value)) return null
  const media: PreviewSceneMediaRef[] = []
  for (const entry of value) {
    const parsed = parseSceneMediaRef(entry)
    if (!parsed) return null
    media.push(parsed)
  }
  return media
}

function parseSceneBlend(value: unknown): PreviewScenePoint | null | undefined {
  if (value === undefined) return undefined
  if (!isRecord(value) || !hasOnlyKeys(value, SCENE_POINT_KEYS)) return null
  if (!isFiniteList([value.x, value.y])) return null
  return { x: value.x as number, y: value.y as number }
}

function parseScenePromptShape(value: unknown): PreviewScenePromptShape | null | undefined {
  if (value === undefined) return undefined
  if (!isRecord(value) || !hasOnlyKeys(value, SCENE_PROMPT_KEYS)) return null
  const record = value as Record<string, unknown>
  if (typeof record.aspect_ratio !== 'string' || !record.aspect_ratio) return null
  if (typeof record.has_inputs !== 'boolean') return null
  return { aspectRatio: record.aspect_ratio, hasInputs: record.has_inputs }
}

function parseSceneRect(value: Record<string, unknown>): {
  x: number
  y: number
  width: number
  height: number
  aspect: number
} | null {
  if (!isFiniteList([value.x, value.y, value.width, value.height, value.aspect])) return null
  const rect = value as unknown as {
    x: number
    y: number
    width: number
    height: number
    aspect: number
  }
  if (!(rect.width > 0 && rect.height > 0 && rect.aspect > 0)) return null
  return { x: rect.x, y: rect.y, width: rect.width, height: rect.height, aspect: rect.aspect }
}

function parseSceneNode(value: unknown): PreviewSceneNode | null {
  if (!isRecord(value) || !hasOnlyKeys(value, SCENE_NODE_KEYS)) return null
  if (typeof value.id !== 'string' || !value.id) return null
  if (value.kind !== 'image' && value.kind !== 'prompt' && value.kind !== 'fusion') return null
  const rect = parseSceneRect(value)
  if (!rect) return null
  const media = parseSceneMediaList(value.media)
  if (!media) return null
  const blend = parseSceneBlend(value.blend)
  if (blend === null) return null
  const prompt = parseScenePromptShape(value.prompt)
  if (prompt === null) return null
  const node: PreviewSceneNode = {
    id: value.id,
    kind: value.kind,
    x: rect.x,
    y: rect.y,
    width: rect.width,
    height: rect.height,
    aspect: rect.aspect,
    media,
  }
  if (blend) node.blend = blend
  if (prompt) node.prompt = prompt
  return node
}

function parseScenePoint(value: unknown): PreviewScenePoint | null {
  if (!isRecord(value) || !hasOnlyKeys(value, SCENE_POINT_KEYS)) return null
  if (!isFiniteList([value.x, value.y])) return null
  return { x: value.x as number, y: value.y as number }
}

function parseSceneConnection(
  value: unknown,
  nodeIds: ReadonlySet<string>,
): PreviewSceneConnection | null {
  if (!isRecord(value) || !hasOnlyKeys(value, SCENE_CONNECTION_KEYS)) return null
  if (
    typeof value.key !== 'string' ||
    typeof value.from !== 'string' ||
    typeof value.to !== 'string'
  ) {
    return null
  }
  if (!nodeIds.has(value.from) || !nodeIds.has(value.to)) return null
  const record = value as Record<string, unknown>
  if (typeof record.from_port !== 'string' || typeof record.to_port !== 'string') return null
  const start = parseScenePoint(value.start)
  const end = parseScenePoint(value.end)
  if (!start || !end) return null
  return {
    key: value.key,
    from: value.from,
    to: value.to,
    fromPort: record.from_port,
    toPort: record.to_port,
    start,
    end,
  }
}

function parseSceneGroup(value: unknown): PreviewSceneGroup | null {
  if (!isRecord(value) || !hasOnlyKeys(value, SCENE_GROUP_KEYS)) return null
  if (typeof value.id !== 'string' || !value.id) return null
  if (
    !Array.isArray(value.members) ||
    !value.members.every((member) => typeof member === 'string')
  ) {
    return null
  }
  if (!isFiniteList([value.x, value.y, value.width, value.height])) return null
  const record = value as Record<string, unknown>
  if (typeof record.is_empty !== 'boolean') return null
  const rect = value as unknown as { x: number; y: number; width: number; height: number }
  return {
    id: value.id,
    members: [...(value.members as string[])],
    x: rect.x,
    y: rect.y,
    width: rect.width,
    height: rect.height,
    isEmpty: record.is_empty,
  }
}

function parseSceneBounds(value: unknown): PreviewSceneBounds | null {
  if (value === null || value === undefined) return value ?? null
  if (!isRecord(value) || !hasOnlyKeys(value, SCENE_BOUNDS_KEYS)) return null
  const candidate = value as Record<string, unknown>
  if (
    !isFiniteList([
      candidate.min_x,
      candidate.min_y,
      candidate.max_x,
      candidate.max_y,
      candidate.width,
      candidate.height,
    ])
  ) {
    return null
  }
  const wire = candidate as unknown as {
    min_x: number
    min_y: number
    max_x: number
    max_y: number
    width: number
    height: number
  }
  if (!(wire.width > 0 && wire.height > 0)) return null
  return {
    minX: wire.min_x,
    minY: wire.min_y,
    maxX: wire.max_x,
    maxY: wire.max_y,
    width: wire.width,
    height: wire.height,
  }
}

function parseSceneExtent(value: unknown): PreviewSceneExtent | null {
  if (!isRecord(value) || !hasOnlyKeys(value, SCENE_EXTENT_KEYS)) return null
  const record = value as Record<string, unknown>
  if (!isFiniteList([record.width, record.height])) return null
  const extent = record as unknown as { width: number; height: number }
  if (extent.width < 0 || extent.height < 0) return null
  return { width: extent.width, height: extent.height }
}

function parseSceneNodeList(
  value: unknown,
): { nodes: PreviewSceneNode[]; nodeIds: Set<string> } | null {
  if (!Array.isArray(value)) return null
  const nodes: PreviewSceneNode[] = []
  const nodeIds = new Set<string>()
  for (const entry of value) {
    const parsed = parseSceneNode(entry)
    if (!parsed || nodeIds.has(parsed.id)) return null
    nodeIds.add(parsed.id)
    nodes.push(parsed)
  }
  return { nodes, nodeIds }
}

function parseSceneConnectionList(
  value: unknown,
  nodeIds: ReadonlySet<string>,
): PreviewSceneConnection[] | null {
  if (!Array.isArray(value)) return null
  const connections: PreviewSceneConnection[] = []
  for (const entry of value) {
    const parsed = parseSceneConnection(entry, nodeIds)
    if (!parsed) return null
    connections.push(parsed)
  }
  return connections
}

function parseSceneGroupList(value: unknown): PreviewSceneGroup[] | null {
  if (!Array.isArray(value)) return null
  const groups: PreviewSceneGroup[] = []
  for (const entry of value) {
    const parsed = parseSceneGroup(entry)
    if (!parsed) return null
    groups.push(parsed)
  }
  return groups
}

/** @externalContract Board Preview Scene wire v1, shared with the Go extractor. */
function parseSceneTail(
  record: Record<string, unknown>,
): { truncated: boolean; budgets: PreviewSceneBudgets; omitted: PreviewSceneOmitted } | null {
  if (!isRecord(record.budgets) || !hasOnlyKeys(record.budgets, SCENE_BUDGET_KEYS)) return null
  if (!isRecord(record.omitted) || !hasOnlyKeys(record.omitted, SCENE_OMITTED_KEYS)) return null
  if (typeof record.truncated !== 'boolean') return null
  const budgets = record.budgets
  const omitted = record.omitted
  if (
    SCENE_BUDGET_KEYS.some(
      (key) =>
        budgets[key] !== null && (!Number.isSafeInteger(budgets[key]) || Number(budgets[key]) < 0),
    )
  )
    return null
  if (
    SCENE_OMITTED_KEYS.some(
      (key) => !Number.isSafeInteger(omitted[key]) || Number(omitted[key]) < 0,
    )
  )
    return null
  return {
    truncated: record.truncated,
    budgets: {
      maxNodes: budgets.max_nodes as number | null,
      maxConnections: budgets.max_connections as number | null,
      maxMediaRefs: budgets.max_media_refs as number | null,
      maxSceneBytes: budgets.max_scene_bytes as number | null,
    },
    omitted: {
      nodes: omitted.nodes as number,
      connections: omitted.connections as number,
      mediaRefs: omitted.media_refs as number,
      groups: omitted.groups as number,
      groupMembers: omitted.group_members as number,
    },
  }
}

// parsePreviewScene strictly parses a wire scene payload (snake_case, the
// same bytes the Go extractor produces). Unknown fields, wrong versions,
// and missing/invalid members return null fail-closed: the parser never
// guesses or fills defaults.
/** @externalContract Board Preview Scene wire v1, shared with the Go extractor. */
export function parsePreviewScene(data: unknown): PreviewScene | null {
  if (!isRecord(data) || !hasOnlyKeys(data, SCENE_WIRE_KEYS)) return null
  if (data.version !== PREVIEW_SCENE_VERSION) return null
  const record = data as Record<string, unknown>
  if (typeof record.content_version !== 'string' || !record.content_version) return null
  const bounds = parseSceneBounds(record.bounds)
  if (bounds === null && record.bounds !== null && record.bounds !== undefined) return null
  const extent = parseSceneExtent(record.extent)
  if (!extent) return null
  const nodeList = parseSceneNodeList(record.nodes)
  if (!nodeList) return null
  const connections = parseSceneConnectionList(record.connections, nodeList.nodeIds)
  if (!connections) return null
  const groups = parseSceneGroupList(record.groups)
  if (!groups) return null
  const tail = parseSceneTail(record)
  if (!tail) return null
  return {
    version: PREVIEW_SCENE_VERSION,
    contentVersion: record.content_version,
    bounds,
    extent,
    nodes: nodeList.nodes,
    connections,
    groups,
    budgets: tail.budgets,
    omitted: tail.omitted,
    truncated: tail.truncated,
  }
}
