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

  test('renders the local member with a self label', () => {
    renderRoster()
    expect(screen.getByTitle('Ada (you)')).toBeVisible()
  })

  test('reuses the page-chrome avatar structure on every non-interactive avatar', () => {
    const view = renderRoster()
    expect(view.container.querySelectorAll('.ib-user-avatar-button')).toHaveLength(3)
    expect(view.container.querySelectorAll('.ib-user-avatar-button__placeholder svg')).toHaveLength(
      3,
    )
    const avatars = view.container.querySelectorAll('.presence-roster__avatar')
    expect(avatars).toHaveLength(3)
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
    ['two remote peers without a local member', [7, 8], null],
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
  ])('hides the cluster when %s', (_label, clientIds, localUser) => {
    renderRoster({
      peers: (clientIds as number[]).map((clientId) => buildPeer({ clientId })),
      localUser: localUser as PresenceUser | null,
    })
    expect(screen.queryByRole('region', { name: 'Online collaborators' })).toBeNull()
  })
})
