import { beforeEach, describe, expect, test } from 'vitest'
import { setUserLocale } from '../../../../../core/i18n'
import { renderWithUser, screen } from '../../../../../test/render'
import PresenceRoster from './PresenceRoster.vue'
import type { PresenceUser, RemotePresencePeer } from '../../../session/collaboration'

function buildPeer(input: {
  clientId: number
  name?: string | undefined
  userId?: string | undefined
}): RemotePresencePeer {
  return {
    clientId: input.clientId,
    state: {
      user: {
        id: input.userId ?? `usr_${input.clientId}`,
        name: input.name ?? `Peer ${input.clientId}`,
      },
      selection: { nodeIds: [] },
    },
  }
}

function renderRoster(input?: {
  peers?: RemotePresencePeer[] | undefined
  localUser?: PresenceUser | null | undefined
}) {
  return renderWithUser(PresenceRoster, {
    props: {
      peers: input?.peers ?? [buildPeer({ clientId: 7 }), buildPeer({ clientId: 8 })],
      localUser: input?.localUser === undefined ? { id: 'usr_ada', name: 'Ada' } : input.localUser,
    },
  })
}

beforeEach(async () => {
  await setUserLocale('en')
})

describe('PresenceRoster', () => {
  test('renders the online member cluster as non-interactive avatars', () => {
    renderRoster()
    expect(screen.getByRole('region', { name: 'Online collaborators' })).toBeVisible()
    expect(screen.getByRole('img', { name: 'Peer 7' })).toBeVisible()
    expect(screen.getByRole('img', { name: 'Peer 8' })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Peer 7' })).toBeNull()
  })

  test('excludes the local user, including their other client sessions', () => {
    renderRoster({
      peers: [
        buildPeer({ clientId: 7 }),
        { ...buildPeer({ clientId: 8, userId: 'usr_ada', name: 'Ada' }), isEditing: true },
      ],
    })
    expect(screen.getAllByRole('img')).toHaveLength(1)
    expect(screen.getByRole('img', { name: 'Peer 7' })).toBeVisible()
    expect(screen.queryByRole('img', { name: /Ada/ })).toBeNull()
  })

  test('reuses the page-chrome avatar structure on every non-interactive avatar', () => {
    const view = renderRoster()
    expect(view.container.querySelectorAll('.ib-user-avatar-button')).toHaveLength(2)
    expect(view.container.querySelectorAll('.ib-user-avatar-button__placeholder svg')).toHaveLength(
      2,
    )
    const avatars = view.container.querySelectorAll('.presence-roster__avatar')
    expect(avatars).toHaveLength(2)
    for (const avatar of avatars) {
      expect(avatar.tagName).toBe('SPAN')
      expect(avatar.classList.contains('ib-user-avatar-button')).toBe(true)
    }
  })

  test('lays out avatars with even spacing', () => {
    const view = renderRoster()
    const stack = view.container.querySelector('.presence-roster__stack')
    expect(stack?.classList.contains('gap-2')).toBe(true)
  })

  test.each([
    ['a 1v1 session', [7], { id: 'usr_ada', name: 'Ada' } as PresenceUser],
    ['one remote peer without a local member', [7], null],
  ])('renders the cluster for %s', (_label, clientIds, localUser) => {
    renderRoster({
      peers: (clientIds as number[]).map((clientId) => buildPeer({ clientId })),
      localUser: localUser as PresenceUser | null,
    })
    expect(screen.getByRole('region', { name: 'Online collaborators' })).toBeVisible()
  })

  test.each([
    ['only the local member is online', [], { id: 'usr_ada', name: 'Ada' } as PresenceUser | null],
    ['no members at all', [], null as PresenceUser | null],
    ['only another local client is online', [7], { id: 'usr_7', name: 'Ada' }],
  ])('hides the cluster when %s', (_label, clientIds, localUser) => {
    renderRoster({
      peers: (clientIds as number[]).map((clientId) => buildPeer({ clientId })),
      localUser: localUser as PresenceUser | null,
    })
    expect(screen.queryByRole('region', { name: 'Online collaborators' })).toBeNull()
  })
})

test('exposes editing status through avatar labels without live announcements', async () => {
  const peer = { ...buildPeer({ clientId: 7, name: 'Bob' }), isEditing: true }
  const view = renderWithUser(PresenceRoster, {
    props: { peers: [peer], localUser: { id: 'usr_ada', name: 'Ada' } },
  })
  expect(screen.getByRole('img', { name: 'Bob · Editing' })).toHaveAttribute(
    'title',
    'Bob · Editing',
  )
  expect(screen.getAllByRole('img')).toHaveLength(1)
  expect(view.container.querySelector('[aria-live]')).toBeNull()
  await view.rerender({ peers: [buildPeer({ clientId: 7, name: 'Bob' })] })
  expect(screen.getByRole('img', { name: 'Bob' })).toHaveAttribute('title', 'Bob')
  expect(screen.getAllByRole('img')).toHaveLength(1)
})

test('shows and hides the cluster as the first collaborator joins and leaves', async () => {
  const view = renderRoster({ peers: [] })
  expect(screen.queryByRole('region', { name: 'Online collaborators' })).toBeNull()
  await view.rerender({ peers: [buildPeer({ clientId: 7 })] })
  expect(screen.getByRole('img', { name: 'Peer 7' })).toBeVisible()
  expect(screen.getAllByRole('img')).toHaveLength(1)
  await view.rerender({ peers: [] })
  expect(screen.queryByRole('region', { name: 'Online collaborators' })).toBeNull()
})
