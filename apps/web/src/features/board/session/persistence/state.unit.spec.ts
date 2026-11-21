import { beforeEach, describe, expect, it, test } from 'vitest'
import { clearStoredAuth, setStoredAuth } from '../../../../core/utils/auth'
import { getWorkspaceSelectionStore, resetWorkspaceSelectionStore } from '../../../workspace'
import { makeBoardState, makePendingImageNodeSnapshot } from '../../testing'
import { scopedBoardStorageKey } from '../catalog/scope'
import {
  deleteBoardState,
  capturePendingBoardTask,
  duplicateBoardState,
  getBoardState,
  replaceBoardStateMap,
  setBoardState,
} from './state'

const WORKSPACE_ID = 'ws_state_spec'
const STORAGE_KEY_PREFIX = 'ideaboard.canvas.boardStates.v2'

function primeScope() {
  setStoredAuth({
    public_id: '00000000-0000-0000-0000-000000000001',
    display_name: 'tester',
    avatar: '',
    role: 'user',
  })
  resetWorkspaceSelectionStore()
  getWorkspaceSelectionStore().select(WORKSPACE_ID)
}

function storageKey(): string {
  const key = scopedBoardStorageKey(STORAGE_KEY_PREFIX)
  if (!key) throw new Error('board scope is not primed')
  return key
}

function readRawEntries(): Record<string, unknown> {
  const raw = localStorage.getItem(storageKey())
  if (!raw) return {}
  return JSON.parse(raw) as Record<string, unknown>
}

function seedRawEntries(entries: Record<string, unknown>) {
  localStorage.setItem(storageKey(), JSON.stringify(entries))
}

function legacyDraft(promptText: string): Record<string, unknown> {
  return {
    ...makeBoardState({ updatedAt: 1_000 }),
    promptNodes: [
      {
        id: 1,
        type: 'prompt',
        x: 0,
        y: 0,
        width: 280,
        text: promptText,
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    ],
    previewSnapshotDataUrl: 'data:image/webp;base64,preview',
    previewSnapshotUpdatedAt: 1_001,
    previewSnapshotVersion: 3,
  }
}

describe('board local persistence', () => {
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
    primeScope()
  })

  test('merges late acceptance into the latest origin draft without changing its timestamp', () => {
    const snapshot = makeBoardState({
      imageNodes: [
        makePendingImageNodeSnapshot({
          pendingTaskId: undefined,
          pendingClientActionId: 'action-origin',
          pendingResultIndex: 0,
        }),
      ],
    })
    const retain = capturePendingBoardTask({
      boardId: 'origin',
      snapshot,
      resultNodeIds: ['image-1'],
      clientActionId: 'action-origin',
    })
    const newer = setBoardState('origin', {
      ...snapshot,
      imageNodes: [{ ...snapshot.imageNodes[0]!, x: 999 }],
    })!
    getWorkspaceSelectionStore().select('ws_other123')

    expect(retain('task_accepted')).toBe(true)
    expect(getBoardState('origin')).toBeNull()
    getWorkspaceSelectionStore().select(WORKSPACE_ID)
    expect(getBoardState('origin')).toMatchObject({
      updatedAt: newer.updatedAt,
      imageNodes: [{ x: 999, pendingTaskId: 'task_accepted' }],
    })
  })

  test.each([
    'deleted-board',
    'deleted-slot',
    'superseded',
    'terminal',
    'conflicting-task',
    'wrong-index',
  ] as const)('does not restore or overwrite a %s after late acceptance', (change) => {
    const node = makePendingImageNodeSnapshot({
      pendingTaskId: undefined,
      pendingClientActionId: 'action-origin',
      pendingResultIndex: 0,
    })
    const snapshot = makeBoardState({ imageNodes: [node] })
    const retain = capturePendingBoardTask({
      boardId: 'origin',
      snapshot,
      resultNodeIds: ['image-1'],
      clientActionId: 'action-origin',
    })
    if (change === 'deleted-board') deleteBoardState('origin')
    else
      setBoardState('origin', {
        ...snapshot,
        imageNodes:
          change === 'deleted-slot'
            ? []
            : [
                {
                  ...node,
                  ...(change === 'superseded' ? { pendingClientActionId: 'action-new' } : {}),
                  ...(change === 'terminal' ? { outputStatus: 'ready' as const } : {}),
                  ...(change === 'conflicting-task' ? { pendingTaskId: 'task_new' } : {}),
                  ...(change === 'wrong-index' ? { pendingResultIndex: 1 } : {}),
                },
              ],
      })
    const before = getBoardState('origin')
    retain('task_accepted')
    expect(getBoardState('origin')).toEqual(before)
  })

  it('rejects drafts that still carry the retired preview cache', () => {
    seedRawEntries({ legacy: legacyDraft('legacy-content') })

    expect(getBoardState('legacy')).toBeNull()
  })

  it('drops undecodable bytes on unrelated writes', () => {
    const legacy = legacyDraft('legacy-content')
    seedRawEntries({ legacy })

    const saved = setBoardState('other', makeBoardState({ updatedAt: 2_000 }))
    expect(saved).not.toBeNull()

    // Strict-only: undecodable leftovers are discarded, not carried along.
    const stored = readRawEntries()
    expect(stored).not.toHaveProperty('legacy')
    expect(stored.other).toEqual(expect.objectContaining({ updatedAt: expect.any(Number) }))
    expect(stored.other).not.toHaveProperty('previewSnapshotDataUrl')
  })

  it('drops removed entries including undecodable ones on replace', () => {
    const legacy = legacyDraft('legacy-content')
    seedRawEntries({ legacy, stale: makeBoardState({ updatedAt: 500 }) })

    replaceBoardStateMap({})

    expect(readRawEntries()).toEqual({})
  })

  it('deletes valid entries explicitly; undecodable ids hold no valid entry', () => {
    replaceBoardStateMap({ valid: makeBoardState({ updatedAt: 2_000 }) })
    seedRawEntries({ ...readRawEntries(), legacy: legacyDraft('legacy-content') })

    expect(deleteBoardState('missing')).toBe(false)
    expect(deleteBoardState('legacy')).toBe(false)
    expect(deleteBoardState('valid')).toBe(true)
    expect(readRawEntries()).not.toHaveProperty('valid')
    // Any write only emits validated content, so the undecodable leftover
    // is discarded along the way instead of being preserved.
    expect(readRawEntries()).not.toHaveProperty('legacy')
  })

  it('duplicates valid drafts without preview metadata', () => {
    replaceBoardStateMap({ source: makeBoardState({ updatedAt: 2_000 }) })

    expect(duplicateBoardState('source', 'target')).toBe(true)
    expect(duplicateBoardState('legacy', 'target')).toBe(false)

    const stored = readRawEntries()
    expect(stored.target).toEqual(
      expect.not.objectContaining({ previewSnapshotDataUrl: expect.anything() }),
    )
  })
})
