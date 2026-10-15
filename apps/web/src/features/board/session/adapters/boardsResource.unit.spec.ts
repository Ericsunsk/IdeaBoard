import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { beforeEach, describe, expect, test, vi } from 'vitest'
import { primeAuthenticatedWorkspace } from '../../testing'
import { fetchWorkspaceBoards } from './boardsResource'

const WORKSPACE_ID = 'ws_test123'

const mocks = vi.hoisted(() => ({
  listBoards: vi.fn(),
  getBoardDocument: vi.fn(),
}))

vi.mock('../../../../api/generated-query/public/boards/boards', () => ({
  listBoards: mocks.listBoards,
  getBoardDocument: mocks.getBoardDocument,
}))

function loadFixtureScene(name: string): unknown {
  const raw = readFileSync(
    join(
      dirname(fileURLToPath(import.meta.url)),
      '../../../../../../api/internal/boards/previewscene/testdata',
      `${name}.json`,
    ),
    'utf8',
  )
  return (JSON.parse(raw) as { scene: unknown }).scene
}

function boardDto(extra: Record<string, unknown> = {}) {
  return {
    id: 'board-1',
    workspace_id: WORKSPACE_ID,
    name: 'Scene Board',
    revision: 3,
    schema_version: 4,
    archived_at: null,
    created_at: new Date(100).toISOString(),
    updated_at: new Date(200).toISOString(),
    ...extra,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  primeAuthenticatedWorkspace(WORKSPACE_ID)
  mocks.listBoards.mockResolvedValue({ data: [], meta: { has_more: false } })
})

describe('boardsResource catalog scenes', () => {
  test('carries the strict-parsed server scene and content version', async () => {
    mocks.listBoards.mockResolvedValue({
      data: [boardDto({ preview_scene: loadFixtureScene('mixed'), content_version: 'cv-1' })],
      meta: { has_more: false },
    })

    const [meta] = await fetchWorkspaceBoards(WORKSPACE_ID)

    expect(meta?.contentVersion).toBe('cv-1')
    // The scene keeps its own verbatim identity (extraction never derives
    // it); the envelope identity rides beside it for skip-re-render.
    expect(meta?.previewScene?.contentVersion).toBe('cv-fixture-mixed-01')
    expect(meta?.previewScene?.nodes.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-2',
      'fusion-4',
    ])
    expect(mocks.getBoardDocument).not.toHaveBeenCalled()
  })

  test('omitted scenes stay null with an empty content version', async () => {
    mocks.listBoards.mockResolvedValue({ data: [boardDto()], meta: { has_more: false } })

    const [meta] = await fetchWorkspaceBoards(WORKSPACE_ID)

    expect(meta?.contentVersion).toBe('')
    expect(meta?.previewScene).toBeNull()
    expect(meta?.revision).toBe(3)
  })

  test('invalid scenes fail closed without breaking the meta', async () => {
    mocks.listBoards.mockResolvedValue({
      data: [
        boardDto({ preview_scene: { version: 2 }, content_version: 'cv-bad' }),
        boardDto({
          id: 'board-2',
          preview_scene: { version: 1, unexpected_scene_field: 'note' },
          content_version: 42,
        }),
      ],
      meta: { has_more: false },
    })

    const metas = await fetchWorkspaceBoards(WORKSPACE_ID)

    expect(metas).toHaveLength(2)
    // The wire identity is carried verbatim when present...
    expect(metas[0]?.contentVersion).toBe('cv-bad')
    expect(metas[0]?.previewScene).toBeNull()
    // ...but a non-string content version never becomes an identity.
    expect(metas[1]?.contentVersion).toBe('')
    expect(metas[1]?.previewScene).toBeNull()
  })
})
