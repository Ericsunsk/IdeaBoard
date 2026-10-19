import { beforeEach, describe, expect, test } from 'vitest'
import { setUserLocale } from '../../../../../core/i18n'
import { renderWithUser, screen } from '../../../../../test/render'
import PresenceRoster from './PresenceRoster.vue'
import type { RemotePresencePeer } from './state'

function buildPeer(input: {
  clientId: number
  name?: string | undefined
  isSpotlighting?: boolean | undefined
}): RemotePresencePeer {
  return {
    clientId: input.clientId,
    state: {
      user: { id: `usr_${input.clientId}`, name: input.name ?? `Peer ${input.clientId}` },
      cursor: { position: { x: 0, y: 0 }, dragging: false },
      selection: { nodeIds: [], marquee: null },
      ...(input.isSpotlighting === true ? { isSpotlighting: true as const } : {}),
    },
  }
}

function renderRoster(input?: {
  peers?: RemotePresencePeer[] | undefined
  followingClientId?: number | null | undefined
  isSpotlighting?: boolean | undefined
  followerCount?: number | undefined
}) {
  return renderWithUser(PresenceRoster, {
    props: {
      peers: input?.peers ?? [buildPeer({ clientId: 7 }), buildPeer({ clientId: 8 })],
      localUser: { id: 'usr_ada', name: 'Ada' },
      followingClientId: input?.followingClientId ?? null,
      isSpotlighting: input?.isSpotlighting ?? false,
      followerCount: input?.followerCount ?? 0,
    },
  })
}

beforeEach(async () => {
  await setUserLocale('en')
})

describe('PresenceRoster', () => {
  test('renders the online member cluster with follow controls', () => {
    renderRoster()
    expect(screen.getByRole('region', { name: 'Online collaborators' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Follow Peer 7' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Follow Peer 8' })).toBeVisible()
  })

  test('toggles follow through the avatar control', async () => {
    const view = renderRoster()
    await view.user.click(screen.getByRole('button', { name: 'Follow Peer 7' }))
    expect(view.emitted()['follow-toggle']).toEqual([[7]])
  })

  test('marks the followed member as pressed with an exit label', async () => {
    const view = renderRoster({ followingClientId: 7 })
    const control = screen.getByRole('button', { name: 'Stop following Peer 7' })
    expect(control).toHaveAttribute('aria-pressed', 'true')
    await view.user.click(control)
    expect(view.emitted()['follow-toggle']).toEqual([[7]])
  })

  test('toggles spotlight and reads the live follower count', async () => {
    const view = renderRoster({ isSpotlighting: true, followerCount: 2 })
    expect(screen.getByText('2 watching')).toBeVisible()
    await view.user.click(screen.getByRole('button', { name: 'Stop spotlight' }))
    expect(view.emitted()['spotlight-toggle']).toHaveLength(1)
  })

  test('hides the cluster without any members', () => {
    renderWithUser(PresenceRoster, {
      props: {
        peers: [],
        localUser: null,
        followingClientId: null,
        isSpotlighting: false,
        followerCount: 0,
      },
    })
    expect(screen.queryByRole('region', { name: 'Online collaborators' })).toBeNull()
  })
})
