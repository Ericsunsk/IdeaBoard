// Gallery scale budgets for Issue #928: large Boards and multi-card
// Galleries must keep bounded payload, projection, and request cost.
//
// Server facts this spec pins client-side (see
// `apps/api/internal/boards/previewscene/contract.md` §Budgets):
// per-card scene ≤ 65_536 bytes, ≤ 500 nodes, ≤ 1_000 connections,
// ≤ 1_500 media refs; truncation keeps full-layout bounds and reports
// `truncated` + `omitted`. The Gallery stays list-only (catalog scenes ride
// the list response; no per-card document fetch), renders at most one
// raster per image node (latest-output-wins), starts no reads for
// unmeasured cards, and reuses one transport read per asset identity
// (dedup + concurrency ≤ 4 live in `thumbnailMedia` / `mediaAssetMetadata`,
// pinned by `thumbnailMedia.unit.spec.ts`).
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { beforeEach, describe, expect, test, vi } from 'vitest'
import { primeAuthenticatedWorkspace } from '../../testing'
import { parsePreviewScene, type PreviewScene } from '../../core/preview/scene'
import { composeSceneThumbnail } from '../../core/preview/renderer'
import { fetchWorkspaceBoards } from '../../session/adapters/boardsResource'

const WORKSPACE_ID = 'ws_test123'
const VIEWPORT = { width: 240, height: 180 }

/** Server per-card scene byte cap (CatalogMaxSceneBytes); change both sides together. */
const GALLERY_SCENE_BYTE_BUDGET = 65_536

const FIXTURE_DIR = join(
  dirname(fileURLToPath(import.meta.url)),
  '../../../../../../api/internal/boards/previewscene/testdata',
)

function loadScene(name: string): PreviewScene {
  const raw = readFileSync(join(FIXTURE_DIR, `${name}.json`), 'utf8')
  const parsed = parsePreviewScene((JSON.parse(raw) as { scene: unknown }).scene)
  if (!parsed) throw new Error(`fixture scene failed to parse: ${name}`)
  return parsed
}

function largeImageScene(nodes: number, assetOf: (index: number) => string): PreviewScene {
  return {
    version: 1,
    contentVersion: 'cv-scale',
    bounds: { minX: 0, minY: 0, maxX: 4000, maxY: 3000, width: 4000, height: 3000 },
    extent: { width: 1, height: 0.75 },
    nodes: Array.from({ length: nodes }, (_, index) => ({
      id: `image-${index}`,
      kind: 'image' as const,
      x: (index % 50) * 0.02,
      y: Math.floor(index / 50) * 0.02,
      width: 0.2,
      height: 0.15,
      aspect: 1.33,
      media: [
        { asset: assetOf(index), assetNumeric: null, tier: 'preview', role: 'output', index: 0 },
      ],
    })),
    connections: Array.from({ length: Math.max(0, nodes - 1) }, (_, index) => ({
      key: `c-${index}`,
      from: `image-${index}`,
      to: `image-${index + 1}`,
      fromPort: 'right',
      toPort: 'left',
      start: { x: 0, y: 0 },
      end: { x: 1, y: 1 },
    })),
    groups: [],
    budgets: {
      maxNodes: 500,
      maxConnections: 1000,
      maxMediaRefs: 1500,
      maxSceneBytes: GALLERY_SCENE_BYTE_BUDGET,
    },
    omitted: { nodes: 0, connections: 0, mediaRefs: 0, groups: 0, groupMembers: 0 },
    truncated: false,
  }
}

const boardMocks = vi.hoisted(() => ({ listBoards: vi.fn(), getBoardDocument: vi.fn() }))

vi.mock('../../../../api/generated-query/public/boards/boards', () => ({
  listBoards: boardMocks.listBoards,
  getBoardDocument: boardMocks.getBoardDocument,
}))

function boardDto(id: string, wireScene: unknown) {
  return {
    id,
    workspace_id: WORKSPACE_ID,
    name: `Board ${id}`,
    revision: 1,
    schema_version: 4,
    creator_user_id: 'usr_creator',
    archived_at: null,
    created_at: new Date(100).toISOString(),
    updated_at: new Date(200).toISOString(),
    content_version: 'cv-scale',
    preview_scene: wireScene,
  }
}

function loadWireScene(name: string): unknown {
  const raw = readFileSync(join(FIXTURE_DIR, `${name}.json`), 'utf8')
  return (JSON.parse(raw) as { scene: unknown }).scene
}

beforeEach(() => {
  vi.clearAllMocks()
  primeAuthenticatedWorkspace(WORKSPACE_ID)
})

describe('gallery scale budgets', () => {
  test('every canonical scene fits the per-card payload budget', () => {
    for (const name of [
      'empty',
      'mixed',
      'image-input-output',
      'fusion',
      'fusion-input-mediaref',
      'groups-connections',
      'theme-switch',
    ]) {
      const bytes = JSON.stringify(loadScene(name)).length
      expect(bytes, `scene ${name}`).toBeLessThanOrEqual(GALLERY_SCENE_BYTE_BUDGET)
    }
    // Typical cards stay far below the cap: the Gallery list cost is
    // dominated by card count, not by single-board size.
    expect(JSON.stringify(loadScene('mixed')).length).toBeLessThan(8_192)
  })

  test('projection stays linear in scene nodes for a capped large scene', () => {
    const scene = largeImageScene(500, (index) => `asset_${index}`)
    const first = composeSceneThumbnail(scene, VIEWPORT)
    expect(composeSceneThumbnail(scene, VIEWPORT)).toEqual(first)
    const rasters = first.nodes.flatMap((node) => node.rasters)
    // Latest-output-wins: one raster per image node, never one per media ref.
    expect(rasters).toHaveLength(500)
    const paintEls = first.nodes.length + first.edges.length + rasters.length
    expect(paintEls).toBeLessThanOrEqual(500 + 499 + 500)
    expect(first.viewBox).toBe('0 0 1 0.75')
  })

  test('duplicate assets keep one raster identity per card instead of per-card reads', () => {
    const scene = largeImageScene(50, () => 'asset_shared')
    const rasters = composeSceneThumbnail(scene, VIEWPORT).nodes.flatMap((node) => node.rasters)
    expect(rasters).toHaveLength(50)
    expect(new Set(rasters.map((raster) => raster.assetId))).toEqual(new Set(['asset_shared']))
  })

  test('unmeasured cards compose frames without rasters', () => {
    const scene = largeImageScene(50, (index) => `asset_${index}`)
    const composed = composeSceneThumbnail(scene, { width: 0, height: 0 })
    expect(composed.nodes).toHaveLength(50)
    expect(composed.nodes.flatMap((node) => node.rasters)).toEqual([])
  })

  test('multi-card hydration stays list-only without per-card document reads', async () => {
    const wireScene = loadWireScene('mixed')
    const cards = Array.from({ length: 50 }, (_, index) => boardDto(`board-${index}`, wireScene))
    boardMocks.listBoards.mockResolvedValue({ data: cards, meta: { has_more: false } })
    const metas = await fetchWorkspaceBoards(WORKSPACE_ID)
    expect(metas).toHaveLength(50)
    expect(metas.every((meta) => meta.previewScene !== null)).toBe(true)
    expect(boardMocks.getBoardDocument).not.toHaveBeenCalled()
    // One bounded page serves all 50 cards: no cursor walk per card.
    expect(boardMocks.listBoards).toHaveBeenCalledTimes(1)
  })
})
