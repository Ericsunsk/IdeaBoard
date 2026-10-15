import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { parsePreviewScene, type PreviewScene } from './scene'
import { composeSceneThumbnail, SCENE_THUMBNAIL_TIER } from './renderer'

const VIEWPORT = { width: 240, height: 180 }

const FIXTURE_DIR = join(
  dirname(fileURLToPath(import.meta.url)),
  '../../../../../../api/internal/boards/previewscene/testdata',
)

type SceneFixture = {
  name: string
  content_version: string
  snapshot: unknown
  scene: unknown
}

function loadScene(name: string): PreviewScene {
  const raw = readFileSync(join(FIXTURE_DIR, `${name}.json`), 'utf8')
  const fixture = JSON.parse(raw) as SceneFixture
  const parsed = parsePreviewScene(fixture.scene)
  if (!parsed) throw new Error(`fixture scene failed to parse: ${name}`)
  return parsed
}

describe('scene thumbnail renderer', () => {
  test('uses a fixed thumbnail tier independent of canvas zoom', () => {
    expect(SCENE_THUMBNAIL_TIER).toBe('preview')
  })

  test('empty boards yield no viewBox and no rasters', () => {
    const scene = loadScene('empty')
    expect(composeSceneThumbnail(scene, VIEWPORT).viewBox).toBeNull()
    expect(composeSceneThumbnail(scene, VIEWPORT).nodes.flatMap((node) => node.rasters)).toEqual([])
  })

  test('image nodes prefer the latest output over the source', () => {
    const scene = loadScene('image-input-output')
    expect(composeSceneThumbnail(scene, VIEWPORT).viewBox).toBe('0 0 0.6666666666666667 1')
    const rasters = composeSceneThumbnail(scene, VIEWPORT).nodes.flatMap((node) => node.rasters)
    expect(rasters).toHaveLength(1)
    expect(rasters[0]).toMatchObject({
      key: 'image-1-output-0',
      assetId: 'asset_out_9',
    })
  })

  test('falls back to the source when no output is displayable', () => {
    const scene = loadScene('image-input-output')
    scene.nodes[0]!.media = scene.nodes[0]!.media.filter((media) => media.role === 'source')
    expect(
      composeSceneThumbnail(scene, VIEWPORT).nodes.flatMap((node) => node.rasters),
    ).toMatchObject([{ assetId: 'asset_src_1' }])
  })

  test('numeric-only media never resolves to a raster', () => {
    const scene = loadScene('groups-connections')
    // image-1 carries a numeric-only source; image-2 carries no media at all.
    expect(composeSceneThumbnail(scene, VIEWPORT).nodes.flatMap((node) => node.rasters)).toEqual([])
    const fusion = loadScene('fusion')
    // fusion-7 inputs and output are all numeric-only: slots keep geometry,
    // but no raster is fabricated for them.
    expect(composeSceneThumbnail(fusion, VIEWPORT).nodes.flatMap((node) => node.rasters)).toEqual(
      [],
    )
    expect(fusion.nodes.find((node) => node.id === 'fusion-7')?.blend).toEqual({
      x: 0.3,
      y: 0.7,
    })
  })

  test('canonical Fusion inputs render without an output', () => {
    const scene = loadScene('fusion-input-mediaref')
    expect(
      composeSceneThumbnail(scene, VIEWPORT).nodes.flatMap((node) => node.rasters),
    ).toMatchObject([{ assetId: 'asset_fusion_input', key: 'fusion-1-input-0' }])
  })

  test.each([0, 1, 4])('Fusion %i inputs align placeholders and rasters', (count) => {
    const scene = loadScene('fusion-input-mediaref')
    const node = scene.nodes[0]!
    const input = node.media[0]!
    node.media = Array.from({ length: count }, (_, index) => ({
      ...input,
      index,
      asset: `asset_${index}`,
    }))
    const view = composeSceneThumbnail(scene, VIEWPORT).nodes[0]!
    expect(view.slots).toHaveLength(count || 2)
    expect(view.spokes).toHaveLength(count || 2)
    expect(view.rasters).toHaveLength(count)
    for (const [index, slot] of view.slots.entries()) {
      expect(slot.x).toBeGreaterThanOrEqual(view.x)
      expect(slot.y).toBeGreaterThanOrEqual(view.y)
      expect(slot.x + slot.width).toBeLessThanOrEqual(view.x + view.width + 1e-9)
      expect(slot.y + slot.height).toBeLessThanOrEqual(view.y + view.height + 1e-9)
      if (view.rasters[index]) expect(view.rasters[index]?.frame).toEqual(slot)
    }
  })

  test('Fusion displayable output replaces input slots, numeric output keeps placeholders', () => {
    const scene = loadScene('fusion-input-mediaref')
    const node = scene.nodes[0]!
    node.media.push({ ...node.media[0]!, role: 'output', asset: 'asset_output' })
    const view = composeSceneThumbnail(scene, VIEWPORT).nodes[0]!
    expect(view.slots).toEqual([])
    expect(view.blend).toBeNull()
    expect(view.rasters).toMatchObject([
      { assetId: 'asset_output', frame: { x: node.x, width: node.width } },
    ])
    node.media[1]!.asset = ''
    node.media[1]!.assetNumeric = 123
    const pending = composeSceneThumbnail(scene, VIEWPORT).nodes[0]!
    expect(pending.slots).toHaveLength(1)
    expect(pending.rasters).toMatchObject([{ assetId: 'asset_fusion_input' }])
  })

  test('connections resolve to right-center to left-center endpoints', () => {
    const scene = loadScene('groups-connections')
    const byId = new Map(scene.nodes.map((node) => [node.id, node]))
    for (const [index, connection] of scene.connections.entries()) {
      const from = byId.get(connection.from)!
      const to = byId.get(connection.to)!
      expect(connection.start).toEqual({ x: from.x + from.width, y: from.y + from.height / 2 })
      expect(connection.end).toEqual({ x: to.x, y: to.y + to.height / 2 })
      const path = composeSceneThumbnail(scene, VIEWPORT).edges[index]!.d
      expect(path.startsWith(`M ${connection.start.x} ${connection.start.y} C`)).toBe(true)
      expect(path.endsWith(`${connection.end.x} ${connection.end.y}`)).toBe(true)
    }
  })

  test('mixed boards keep negative-coord shift, overlap order, and aspect', () => {
    const scene = loadScene('mixed')
    expect(composeSceneThumbnail(scene, VIEWPORT).viewBox).toBe('0 0 1 0.81')
    const view = composeSceneThumbnail(scene, VIEWPORT)
    expect(view.nodes.flatMap((node) => node.rasters).map((raster) => raster.assetId)).toEqual([
      'asset_out_m',
    ])
    for (const node of scene.nodes) {
      expect(node.x).toBeGreaterThanOrEqual(0)
      expect(node.y).toBeGreaterThanOrEqual(0)
    }
    expect(scene.nodes.map((node) => node.id)).toEqual(['image-1', 'prompt-2', 'fusion-4'])
    expect(view.edges).toHaveLength(2)
  })

  test('theme-switch scenes project identically to mixed scenes', () => {
    expect(composeSceneThumbnail(loadScene('theme-switch'), VIEWPORT)).toEqual(
      composeSceneThumbnail(loadScene('mixed'), VIEWPORT),
    )
  })

  test('prompt nodes never expose text to the thumbnail view-model', () => {
    const scenes = ['mixed', 'fusion', 'theme-switch'].map(loadScene)
    const payload = JSON.stringify(
      scenes.map((scene) => composeSceneThumbnail(scene, VIEWPORT)),
    ).toLowerCase()
    expect(payload).not.toContain('secret')
    for (const scene of scenes) {
      expect(
        composeSceneThumbnail(scene, VIEWPORT)
          .nodes.filter((node) => node.kind === 'prompt')
          .every((node) => node.rasters.length === 0),
      ).toBe(true)
    }
  })

  test('server budget truncation passes through with surviving geometry', () => {
    const scene = loadScene('mixed')
    scene.nodes = scene.nodes.slice(0, 1)
    scene.connections = []
    scene.groups = []
    scene.truncated = true
    scene.budgets.maxNodes = 1
    scene.omitted.nodes = 2
    expect(scene.truncated).toBe(true)
    expect(scene.nodes.map((node) => node.id)).toEqual(['image-1'])
    expect(scene.connections).toEqual([])
    expect(composeSceneThumbnail(scene, VIEWPORT).viewBox).toBe('0 0 1 0.81')
    expect(
      composeSceneThumbnail(scene, VIEWPORT).nodes.flatMap((node) => node.rasters),
    ).toHaveLength(1)
  })

  test('every discernible image remains eligible beyond six, in Scene paint order', () => {
    const scene = loadScene('image-input-output')
    const image = scene.nodes[0]!
    scene.extent = { width: 1, height: 1 }
    scene.nodes = Array.from({ length: 30 }, (_, index) => ({
      ...image,
      id: `image-${index + 1}`,
      x: (index % 6) / 6,
      y: Math.floor(index / 6) / 5,
      width: 0.15,
      height: 0.18,
      media: [{ ...image.media[0]!, asset: `asset_board_${index + 1}` }],
    }))
    const view = composeSceneThumbnail(scene, VIEWPORT)
    expect(view.nodes).toHaveLength(30)
    expect(view.nodes.flatMap((node) => node.rasters).map((raster) => raster.assetId)).toEqual(
      scene.nodes.map((_, index) => `asset_board_${index + 1}`),
    )
  })

  test.each([
    { width: 0, height: 180 },
    { width: 240, height: 0 },
    { width: -1, height: 180 },
    { width: NaN, height: 180 },
    { width: 240, height: Infinity },
  ])('keeps geometry without rasters for invalid viewport $width x $height', (viewport) => {
    const scene = loadScene('mixed')
    const view = composeSceneThumbnail(scene, viewport)
    expect(view.nodes).toHaveLength(scene.nodes.length)
    expect(view.edges).toHaveLength(scene.connections.length)
    expect(view.groups).toHaveLength(scene.groups.length)
    expect(view.nodes.flatMap((node) => node.rasters)).toEqual([])
  })

  test.each([
    { extent: { width: 4, height: 1 }, width: 0.2, height: 0.5 },
    { extent: { width: 1, height: 4 }, width: 0.5, height: 0.3 },
    { extent: { width: 1, height: 1 }, width: 0.01, height: 1 },
    { extent: { width: 1, height: 1 }, width: 1, height: 0.01 },
  ])('filters small or thin rasters using meet scaling: %o', ({ extent, width, height }) => {
    const scene = loadScene('image-input-output')
    scene.extent = extent
    Object.assign(scene.nodes[0]!, { width, height })
    const view = composeSceneThumbnail(scene, VIEWPORT)
    expect(view.nodes[0]!.rasters).toEqual([])
    expect(view.nodes[0]!).toMatchObject({ width, height })
    expect(view.nodes[0]!.glyphTransform).not.toBeNull()
  })

  test('includes both dimensions at 16 CSS px and re-evaluates with card resize', () => {
    const scene = loadScene('image-input-output')
    scene.extent = { width: 1, height: 1 }
    Object.assign(scene.nodes[0]!, { width: 0.1, height: 0.1 })
    expect(composeSceneThumbnail(scene, { width: 240, height: 159 }).nodes[0]!.rasters).toEqual([])
    const view = composeSceneThumbnail(scene, { width: 240, height: 160 })
    expect(view.nodes[0]!.rasters).toHaveLength(1)
    expect(composeSceneThumbnail(scene, VIEWPORT).nodes[0]!.rasters[0]!.key).toBe(
      view.nodes[0]!.rasters[0]!.key,
    )
  })

  test('measures Fusion slots rather than admitting them by the parent frame', () => {
    const scene = loadScene('fusion-input-mediaref')
    scene.extent = { width: 1, height: 1 }
    const node = scene.nodes[0]!
    Object.assign(node, { width: 0.2, height: 0.2 })
    node.media = Array.from({ length: 4 }, (_, index) => ({ ...node.media[0]!, index }))
    const view = composeSceneThumbnail(scene, VIEWPORT).nodes[0]!
    expect(view.rasters).toEqual([])
    expect(view.slots).toHaveLength(4)
    expect(view.spokes).toHaveLength(4)
    expect(
      composeSceneThumbnail(scene, { width: 480, height: 360 }).nodes[0]!.rasters,
    ).toHaveLength(4)
  })

  test('shared fixture payloads stay within the recorded client budgets', () => {
    const names = ['empty', 'image-input-output', 'fusion', 'groups-connections', 'mixed']
    let maxBytes = 0
    let maxMedia = 0
    for (const name of names) {
      const raw = readFileSync(join(FIXTURE_DIR, `${name}.json`), 'utf8')
      const fixture = JSON.parse(raw) as SceneFixture
      const bytes = Buffer.byteLength(JSON.stringify(fixture.scene), 'utf8')
      maxBytes = Math.max(maxBytes, bytes)
      const scene = loadScene(name)
      maxMedia = Math.max(
        maxMedia,
        scene.nodes.reduce((sum, node) => sum + node.media.length, 0),
      )
      expect(fixture.content_version).toBeTruthy()
    }
    expect(maxBytes).toBeLessThanOrEqual(2048)
    expect(maxMedia).toBeLessThanOrEqual(8)
  })
})
