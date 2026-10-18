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
  test('partitions owned and selected Boards without inherited Workspace access', () => {
    const own = makeBoardMeta('own', 'Own', 1, {
      revision: 1,
      isCreator: true,
      hasDirectGrant: false,
      workspaceAccess: 'editor',
    })
    const selected = makeBoardMeta('selected', 'Selected', 1, {
      revision: 1,
      isCreator: false,
      hasDirectGrant: true,
      workspaceAccess: 'viewer',
    })
    const unchecked = makeBoardMeta('unchecked', 'Unchecked', 1, {
      revision: 1,
      isCreator: false,
      hasDirectGrant: false,
      workspaceAccess: 'editor',
    })
    expect(
      filterBoardsByRange([own, selected, unchecked], 'personal').map((board) => board.id),
    ).toEqual(['own'])
    expect(
      filterBoardsByRange([own, selected, unchecked], 'team').map((board) => board.id),
    ).toEqual(['selected'])
    expect(isTeamBoard(own)).toBe(false)
    expect(isPersonalBoard(selected)).toBe(false)
  })

  test('Off does not place a formerly selected Board in either range', () => {
    const off = makeBoardMeta('off', 'Off', 1, {
      revision: 1,
      isCreator: false,
      hasDirectGrant: false,
      workspaceAccess: 'none',
    })
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
