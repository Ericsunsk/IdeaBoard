import { beforeEach, describe, expect, test } from 'vitest'
import { setUserLocale } from '../../../../core/i18n'
import { renderWithUser, screen } from '../../../../test/render'
import { makeSessionUser } from '../../../../test/factories'
import type { BoardMeta } from '../../session/catalog/boards'
import { makeBoardMeta } from '../../testing'
import BoardCard from './BoardCard.vue'

function renderCard(
  board: BoardMeta,
  currentUser?: { public_id?: string | null; avatar?: string | null } | null,
) {
  return renderWithUser(BoardCard, {
    props: {
      board,
      sceneEntry: null,
      workspaceId: null,
      isRenaming: false,
      renameDraft: '',
      isMenuOpen: true,
      displayDate: 'today',
      ...(currentUser !== undefined ? { currentUser } : {}),
    },
  })
}

function teamBoard(overrides: Partial<BoardMeta> = {}): BoardMeta {
  return makeBoardMeta('board-1', 'Campaign', 2, {
    revision: 1,
    accessState: 'workspace',
    workspaceAccess: 'viewer',
    isCreator: false,
    hasDirectGrant: true,
    capabilities: { read: true, write: false, manage: false, generate: false },
    creatorUserId: 'usr_alice',
    ...overrides,
  })
}

beforeEach(async () => {
  await setUserLocale('en')
})

describe('BoardCard owner avatar', () => {
  test('renders the owner avatar image on team cards', () => {
    const view = renderCard(teamBoard({ ownerAvatarUrl: '/avatar-alice.png' }))

    const img = view.container.querySelector('img')
    expect(img?.getAttribute('src')).toBe('/avatar-alice.png')
    expect(img?.getAttribute('alt')).toBe('usr_alice')
    // Team cards pair the avatar with the Workspace text (no icon).
    expect(screen.getByText('Workspace')).toBeVisible()
  })

  test('sizes public-route avatar URLs for the badge', () => {
    const view = renderCard(teamBoard({ ownerAvatarUrl: '/api/v1/public/users/AVTR01/avatar?v=2' }))

    expect(view.container.querySelector('img')?.getAttribute('src')).toBe(
      '/api/v1/public/users/AVTR01/avatar?v=2&size=64',
    )
  })

  test('falls back to the creator initial without an avatar URL', async () => {
    const view = renderCard(teamBoard())

    expect(view.container.querySelector('img')).toBeNull()
    // Reka renders the fallback after its delay timer, so await it.
    expect(await screen.findByText('A')).toBeVisible()
  })

  test('falls back to the viewer session avatar for the self entry', () => {
    const view = renderCard(
      teamBoard(),
      makeSessionUser({
        public_id: 'usr_alice',
        avatar: '/api/v1/public/users/usr_alice/avatar?v=6',
      }),
    )
    expect(view.container.querySelector('img')?.getAttribute('src')).toBe(
      '/api/v1/public/users/usr_alice/avatar?v=6&size=64',
    )
  })

  test('renders the badge with no avatar on private cards', () => {
    const view = renderCard(
      teamBoard({
        accessState: 'private',
        workspaceAccess: 'none',
        isCreator: true,
        hasDirectGrant: false,
        ownerAvatarUrl: '/avatar-alice.png',
      }),
    )

    // Private posture keeps the icon + text badge with no owner avatar.
    expect(screen.getByText('Private')).toBeVisible()
    expect(view.container.querySelector('img')).toBeNull()
    expect(view.container.querySelector('.size-5')).toBeNull()
    expect(screen.queryByText('A')).toBeNull()
  })

  test('renders the badge with no avatar without a creator identity', () => {
    const board = teamBoard()
    delete board.creatorUserId
    const view = renderCard(board)

    expect(screen.getByText('Workspace')).toBeVisible()
    expect(view.container.querySelector('img')).toBeNull()
  })
})
