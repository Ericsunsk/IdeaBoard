import { beforeEach, describe, expect, test } from 'vitest'
import { setStoredAuth } from '../../../../core/utils/auth'
import { getWorkspaceSelectionStore } from '../../../workspace'
import { makeBoardMeta, primeAuthenticatedWorkspace } from '../../testing'
import {
  filterBoardsByRange,
  isPersonalBoard,
  isTeamBoard,
  persistGalleryRange,
  readPersistedGalleryRange,
} from './range'

beforeEach(() => {
  localStorage.clear()
  primeAuthenticatedWorkspace('ws_range1')
})

describe('gallery range derivation', () => {
  test('overlaps creator-shared boards in personal and team', () => {
    const own = makeBoardMeta('own', 'Own', 1, {
      revision: 1,
      accessState: 'private',
      isCreator: true,
      hasDirectGrant: false,
      workspaceAccess: 'none',
    })
    const selected = makeBoardMeta('selected', 'Selected', 1, {
      revision: 1,
      accessState: 'workspace',
      isCreator: false,
      hasDirectGrant: true,
      workspaceAccess: 'viewer',
    })
    // Creator-shared: the owner stays in the catalog with
    // access_state=workspace, so it belongs to both ranges (overlap
    // semantics: personal is ownership, team is workspace-shared).
    const sharedBySelf = makeBoardMeta('shared-by-self', 'Shared By Self', 1, {
      revision: 1,
      accessState: 'workspace',
      isCreator: true,
      hasDirectGrant: false,
      workspaceAccess: 'editor',
    })
    expect(
      filterBoardsByRange([own, selected, sharedBySelf], 'personal').map((board) => board.id),
    ).toEqual(['own', 'shared-by-self'])
    expect(
      filterBoardsByRange([own, selected, sharedBySelf], 'team').map((board) => board.id),
    ).toEqual(['selected', 'shared-by-self'])
    expect(isTeamBoard(own)).toBe(false)
    expect(isPersonalBoard(selected)).toBe(false)
    expect(isTeamBoard(sharedBySelf)).toBe(true)
    expect(isPersonalBoard(sharedBySelf)).toBe(true)
  })

  test('requires server access_state before including a board in team', () => {
    const selected = makeBoardMeta('selected', 'Selected', 1, {
      revision: 1,
      isCreator: false,
      hasDirectGrant: true,
      workspaceAccess: 'viewer',
    })
    expect(isTeamBoard(selected)).toBe(false)
  })

  test('ranges overlap on creator-shared boards and filter independently', () => {
    const boards = [
      makeBoardMeta('own', 'Own', 1, {
        revision: 1,
        accessState: 'private',
        isCreator: true,
        hasDirectGrant: false,
        workspaceAccess: 'none',
      }),
      makeBoardMeta('selected', 'Selected', 1, {
        revision: 1,
        accessState: 'workspace',
        isCreator: false,
        hasDirectGrant: true,
        workspaceAccess: 'viewer',
      }),
      makeBoardMeta('shared-by-self', 'Shared By Self', 1, {
        revision: 1,
        accessState: 'workspace',
        isCreator: true,
        hasDirectGrant: false,
        workspaceAccess: 'editor',
      }),
      makeBoardMeta('off', 'Off', 1, {
        revision: 1,
        accessState: 'private',
        isCreator: false,
        hasDirectGrant: false,
        workspaceAccess: 'none',
      }),
      makeBoardMeta('draft', 'Draft', 1),
    ]
    for (const board of boards) {
      if (board.id === 'shared-by-self') {
        // The overlap: creator-shared boards appear in both ranges.
        expect(isPersonalBoard(board)).toBe(true)
        expect(isTeamBoard(board)).toBe(true)
      } else if (board.id === 'off') {
        // Synced private boards the caller neither created nor holds a
        // grant for never occur in the authorized catalog; under ownership
        // semantics they land in neither range.
        expect(isPersonalBoard(board)).toBe(false)
        expect(isTeamBoard(board)).toBe(false)
      } else {
        expect(isPersonalBoard(board) !== isTeamBoard(board)).toBe(true)
      }
    }
    const personalIds = filterBoardsByRange(boards, 'personal').map((board) => board.id)
    const teamIds = filterBoardsByRange(boards, 'team').map((board) => board.id)
    // Ranges are non-exclusive counts: personal is ownership
    // (isCreator or unsynced draft), team is workspace-shared.
    expect(personalIds.sort()).toEqual(['draft', 'own', 'shared-by-self'])
    expect(teamIds.sort()).toEqual(['selected', 'shared-by-self'])
  })

  test('synced private boards without a creator flag land in neither range', () => {
    const off = makeBoardMeta('off', 'Off', 1, {
      revision: 1,
      accessState: 'private',
      isCreator: false,
      hasDirectGrant: false,
      workspaceAccess: 'none',
    })
    // Unreachable in the authorized catalog (a synced private board the
    // caller neither created nor holds a grant for is purged); ownership
    // semantics keep it out of both ranges.
    expect(isPersonalBoard(off)).toBe(false)
    expect(isTeamBoard(off)).toBe(false)
  })

  test('keeps never-synced local drafts in personal', () => {
    const draft = makeBoardMeta('draft')
    expect(isPersonalBoard(draft)).toBe(true)
    expect(isTeamBoard(draft)).toBe(false)
  })
})

describe('gallery range persistence', () => {
  test('defaults to personal and isolates the selection per User + Workspace', () => {
    expect(readPersistedGalleryRange()).toBe('personal')

    persistGalleryRange('team')
    expect(readPersistedGalleryRange()).toBe('team')

    getWorkspaceSelectionStore().select('ws_range2')
    expect(readPersistedGalleryRange()).toBe('personal')

    getWorkspaceSelectionStore().select('ws_range1')
    expect(readPersistedGalleryRange()).toBe('team')

    setStoredAuth({
      public_id: '00000000-0000-0000-0000-000000000002',
      display_name: 'other',
      avatar: '',
      role: 'user',
    })
    expect(readPersistedGalleryRange()).toBe('personal')
  })

  test('falls back to personal for invalid stored values', () => {
    persistGalleryRange('team')
    expect(readPersistedGalleryRange()).toBe('team')
    for (let index = 0; index < window.localStorage.length; index += 1) {
      const key = window.localStorage.key(index)
      if (key?.startsWith('ideaboard.board.galleryRange.v1.')) {
        window.localStorage.setItem(key, '"everyone"')
      }
    }
    expect(readPersistedGalleryRange()).toBe('personal')
  })
})
