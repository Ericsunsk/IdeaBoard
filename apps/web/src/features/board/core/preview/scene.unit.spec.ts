import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { parsePreviewScene } from './scene'

const FIXTURE_DIR = join(
  dirname(fileURLToPath(import.meta.url)),
  '../../../../../../api/internal/boards/previewscene/testdata',
)
const FIXTURE_NAMES = [
  'empty',
  'image-input-output',
  'fusion',
  'fusion-input-mediaref',
  'groups-connections',
  'mixed',
  'theme-switch',
] as const

function loadWire(name = 'mixed') {
  return JSON.parse(readFileSync(join(FIXTURE_DIR, `${name}.json`), 'utf8')).scene
}

describe('parsePreviewScene', () => {
  for (const name of FIXTURE_NAMES) {
    test(`consumes the shared ${name} wire fixture`, () => {
      const wire = loadWire(name)
      const scene = parsePreviewScene(wire)
      expect(scene).not.toBeNull()
      expect(scene?.contentVersion).toBe(wire.content_version)
      expect(scene?.nodes.map((node) => node.id)).toEqual(
        wire.nodes.map((node: { id: string }) => node.id),
      )
      expect(scene?.connections).toHaveLength(wire.connections.length)
      expect(scene?.groups).toHaveLength(wire.groups.length)
      expect(scene?.extent).toEqual(wire.extent)
      expect(scene?.omitted).toEqual({
        nodes: 0,
        connections: 0,
        mediaRefs: 0,
        groups: 0,
        groupMembers: 0,
      })
    })
  }

  test('preserves server budgets, omission counts, and full-layout bounds', () => {
    const wire = loadWire()
    const budgeted = {
      ...wire,
      nodes: wire.nodes.slice(0, 1),
      connections: [],
      groups: [],
      budgets: { max_nodes: 1, max_connections: 2, max_media_refs: 3, max_scene_bytes: 65536 },
      omitted: { nodes: 2, connections: 2, media_refs: 3, groups: 1, group_members: 2 },
    }
    budgeted.truncated = true
    expect(parsePreviewScene(budgeted)?.truncated).toBe(true)
    expect(parsePreviewScene(budgeted)).toMatchObject({
      bounds: {
        minX: wire.bounds.min_x,
        minY: wire.bounds.min_y,
        width: wire.bounds.width,
        height: wire.bounds.height,
      },
      extent: wire.extent,
      budgets: { maxNodes: 1, maxConnections: 2, maxMediaRefs: 3, maxSceneBytes: 65536 },
      omitted: { nodes: 2, connections: 2, mediaRefs: 3, groups: 1, groupMembers: 2 },
    })
  })

  test('rejects unknown versions, unknown fields, and missing required data', () => {
    const wire = loadWire()
    expect(parsePreviewScene({ ...wire, version: 2 })).toBeNull()
    expect(parsePreviewScene({ ...wire, unexpected_field: 'note' })).toBeNull()
    expect(parsePreviewScene({ ...wire, content_version: '' })).toBeNull()
    expect(parsePreviewScene({ ...wire, extent: undefined })).toBeNull()
    expect(parsePreviewScene({ ...wire, budgets: {} })).toBeNull()
    expect(parsePreviewScene(null)).toBeNull()
  })

  test('rejects malformed nested fields, duplicate nodes, and dangling connections', () => {
    const wire = loadWire()
    expect(parsePreviewScene({ ...wire, nodes: [{ ...wire.nodes[0], x: NaN }] })).toBeNull()
    expect(parsePreviewScene({ ...wire, nodes: [{ ...wire.nodes[0], text: 'secret' }] })).toBeNull()
    expect(parsePreviewScene({ ...wire, nodes: [wire.nodes[0], wire.nodes[0]] })).toBeNull()
    expect(
      parsePreviewScene({ ...wire, connections: [{ ...wire.connections[0], to: 'missing' }] }),
    ).toBeNull()
    expect(parsePreviewScene({ ...wire, budgets: { ...wire.budgets, max_nodes: -1 } })).toBeNull()
    expect(parsePreviewScene({ ...wire, omitted: { ...wire.omitted, groups: 0.5 } })).toBeNull()
  })
})
