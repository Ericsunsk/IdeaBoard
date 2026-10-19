import { beforeEach, describe, expect, test } from 'vitest'
import { setUserLocale } from '../../../../../core/i18n'
import { renderWithUser, screen } from '../../../../../test/render'
import PresenceRoster from './PresenceRoster.vue'
import type { PresenceUser, RemotePresencePeer } from '../../../session/collaboration'

function buildPeer(input: { clientId: number; name?: string | undefined }): RemotePresencePeer {
  return {
    clientId: input.clientId,
    state: {
      user: { id: `usr_${input.clientId}`, name: input.name ?? `Peer ${input.clientId}` },
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

  test('renders the local member without a presence dot', () => {
    renderRoster()
    expect(screen.getByTitle('Ada (you)')).toBeVisible()
  })

  test('reuses the page-chrome avatar structure with icon placeholders', () => {
    const view = renderRoster()
    expect(view.container.querySelectorAll('.ib-user-avatar-button')).toHaveLength(3)
    expect(view.container.querySelectorAll('.ib-user-avatar-button__placeholder svg')).toHaveLength(
      3,
    )
    expect(view.container.querySelector('.presence-roster__initial')).toBeNull()
  })

  test('keeps a single separator-ring hook on every non-interactive avatar', () => {
    const view = renderRoster()
    const avatars = view.container.querySelectorAll('.presence-roster__avatar')
    expect(avatars).toHaveLength(3)
    for (const avatar of avatars) {
      expect(avatar.tagName).toBe('SPAN')
      expect(avatar.classList.contains('ib-user-avatar-button')).toBe(true)
    }
  })

  test('lays out avatars with even spacing and no overlap hook', () => {
    const view = renderRoster()
    const stack = view.container.querySelector('.presence-roster__stack')
    expect(stack?.classList.contains('gap-2')).toBe(true)
  })

  test('renders the cluster for a 1v1 session', () => {
    renderRoster({ peers: [buildPeer({ clientId: 7 })] })
    expect(screen.getByRole('region', { name: 'Online collaborators' })).toBeVisible()
    expect(screen.getByRole('img', { name: 'Peer 7' })).toBeVisible()
  })

  test('renders the cluster when only two remote peers are online', () => {
    renderRoster({
      peers: [buildPeer({ clientId: 7 }), buildPeer({ clientId: 8 })],
      localUser: null,
    })
    expect(screen.getByRole('region', { name: 'Online collaborators' })).toBeVisible()
  })

  test('hides the cluster when only the local member is online', () => {
    renderRoster({ peers: [] })
    expect(screen.queryByRole('region', { name: 'Online collaborators' })).toBeNull()
  })

  test('renders no broadcast control', () => {
    renderRoster()
    expect(screen.queryByRole('button')).toBeNull()
  })

  test('hides the cluster without any members', () => {
    renderWithUser(PresenceRoster, {
      props: {
        peers: [],
        localUser: null,
      },
    })
    expect(screen.queryByRole('region', { name: 'Online collaborators' })).toBeNull()
  })
})
