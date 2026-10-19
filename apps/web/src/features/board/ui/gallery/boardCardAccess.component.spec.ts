import { afterEach, beforeEach, describe, expect, test } from 'vitest'
import { setUserLocale } from '../../../../core/i18n'
import { renderWithUser, screen } from '../../../../test/render'
import type { BoardAccessCapabilities, BoardMeta } from '../../session/catalog/boards'
import { makeBoardMeta } from '../../testing'
import BoardCard from './BoardCard.vue'

const ownerCapabilities: BoardAccessCapabilities = {
  read: true,
  write: true,
  manage: true,
  generate: true,
}
const viewerCapabilities: BoardAccessCapabilities = {
  read: true,
  write: false,
  manage: false,
  generate: false,
}

function renderCard(board: BoardMeta) {
  return renderWithUser(BoardCard, {
    props: {
      board,
      sceneEntry: null,
      workspaceId: null,
      isRenaming: false,
      renameDraft: '',
      isMenuOpen: true,
      displayDate: 'today',
    },
  })
}

function svgCount(view: ReturnType<typeof renderCard>): number {
  return view.container.querySelectorAll('svg').length
}

beforeEach(async () => {
  await setUserLocale('en')
})

afterEach(async () => {
  await setUserLocale('en')
})

describe('BoardCard access posture', () => {
  test.each([
    ['private', 'Private', 2],
    ['workspace', 'Workspace', 1],
  ] as const)('renders the %s posture as text', (accessState, copy, expectedSvg) => {
    const view = renderCard(
      makeBoardMeta('board-1', 'Campaign', 2, {
        revision: 1,
        accessState,
        workspaceAccess: accessState === 'workspace' ? 'viewer' : 'none',
        isCreator: true,
        hasDirectGrant: false,
        capabilities: ownerCapabilities,
      }),
    )

    expect(screen.getByText(copy)).toBeVisible()
    // Private pairs the text with the badge icon (badge svg + menu svg);
    // workspace pairs the text with the owner avatar and carries no badge
    // icon (these boards carry no creator identity so no avatar renders).
    expect(svgCount(view)).toBe(expectedSvg)
  })

  test('shows no badge before the server mints access facts', () => {
    const view = renderCard(makeBoardMeta('draft', 'Draft', 2))

    expect(screen.queryByText('Private')).toBeNull()
    expect(screen.queryByText('Workspace')).toBeNull()
    expect(svgCount(view)).toBe(1)
  })
})

describe('BoardCard capability gating', () => {
  test('limits viewers to duplicating', async () => {
    const view = renderCard(
      makeBoardMeta('board-1', 'Campaign', 2, {
        revision: 1,
        accessState: 'workspace',
        workspaceAccess: 'viewer',
        isCreator: false,
        hasDirectGrant: false,
        capabilities: viewerCapabilities,
      }),
    )

    await view.user.click(screen.getByRole('button', { name: 'More actions' }))
    expect(await screen.findByRole('menuitem', { name: 'Duplicate' })).toBeVisible()
    expect(screen.queryByRole('menuitem', { name: 'Rename' })).toBeNull()
    expect(screen.queryByRole('menuitem', { name: 'Share' })).toBeNull()
    expect(screen.queryByRole('menuitem', { name: 'Delete' })).toBeNull()
  })

  test('offers rename, duplicate, share, and delete to managers', async () => {
    const view = renderCard(
      makeBoardMeta('board-1', 'Campaign', 2, {
        revision: 1,
        accessState: 'private',
        workspaceAccess: 'none',
        isCreator: true,
        hasDirectGrant: false,
        capabilities: ownerCapabilities,
      }),
    )

    await view.user.click(screen.getByRole('button', { name: 'More actions' }))
    expect(await screen.findByRole('menuitem', { name: 'Rename' })).toBeVisible()
    expect(screen.getByRole('menuitem', { name: 'Duplicate' })).toBeVisible()
    expect(screen.getByRole('menuitem', { name: 'Share' })).toBeVisible()
    expect(screen.getByRole('menuitem', { name: 'Delete' })).toBeVisible()
  })

  test('keeps local actions on unsynced drafts while hiding share', async () => {
    const view = renderCard(makeBoardMeta('draft', 'Draft', 2))

    await view.user.click(screen.getByRole('button', { name: 'More actions' }))
    expect(await screen.findByRole('menuitem', { name: 'Rename' })).toBeVisible()
    expect(screen.getByRole('menuitem', { name: 'Duplicate' })).toBeVisible()
    expect(screen.getByRole('menuitem', { name: 'Delete' })).toBeVisible()
    expect(screen.queryByRole('menuitem', { name: 'Share' })).toBeNull()
  })

  test('emits the share intent for the Share surface', async () => {
    const view = renderCard(
      makeBoardMeta('board-1', 'Campaign', 2, {
        revision: 1,
        accessState: 'private',
        workspaceAccess: 'none',
        isCreator: true,
        hasDirectGrant: false,
        capabilities: ownerCapabilities,
      }),
    )

    await view.user.click(screen.getByRole('button', { name: 'More actions' }))
    await view.user.click(await screen.findByRole('menuitem', { name: 'Share' }))

    expect(view.emitted()['share-action']).toHaveLength(1)
  })
})
