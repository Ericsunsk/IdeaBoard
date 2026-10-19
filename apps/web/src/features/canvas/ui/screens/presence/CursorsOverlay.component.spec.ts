import { afterEach, beforeEach, describe, expect, test } from 'vitest'
import { setUserLocale } from '../../../../../core/i18n'
import { renderWithUser, screen } from '../../../../../test/render'
import { makeSessionUser } from '../../../../../test/factories'
import { useUserStore } from '../../../../auth'
import PresenceCursorsOverlay from './CursorsOverlay.vue'
import type { RemotePresencePeer } from '../../../session/collaboration'

function buildPeer(overrides: Partial<RemotePresencePeer> = {}): RemotePresencePeer {
  return {
    clientId: 7,
    state: {
      user: { id: 'usr_bob', name: 'Bob' },
      cursor: { position: { x: 10, y: 20 }, dragging: false },
      selection: { nodeIds: [], marquee: null },
    },
    ...overrides,
  }
}

function renderOverlay(peers: RemotePresencePeer[]) {
  return renderWithUser(PresenceCursorsOverlay, {
    props: { peers, viewport: { x: 100, y: 50, zoom: 2 } },
  })
}

beforeEach(async () => {
  await setUserLocale('en')
  useUserStore().clearAuth()
})

afterEach(() => {
  useUserStore().clearAuth()
})

describe('CursorsOverlay', () => {
  test('positions the remote cursor through the viewport transform', () => {
    const view = renderOverlay([buildPeer()])

    expect(screen.getByText('Bob')).toBeVisible()
    const peer = view.container.querySelector('.presence-cursors__peer') as HTMLElement | null
    expect(peer?.style.left).toBe('120px')
    expect(peer?.style.top).toBe('90px')
  })

  test('hides peers whose pointer left the canvas', () => {
    const view = renderOverlay([
      buildPeer({
        state: {
          user: { id: 'usr_bob', name: 'Bob' },
          cursor: null,
          selection: { nodeIds: [], marquee: null },
        },
      }),
    ])

    expect(screen.queryByText('Bob')).toBeNull()
    expect(view.container.querySelector('.presence-cursors__peer')).toBeNull()
  })

  test('renders the peer avatar image when shared', () => {
    const view = renderOverlay([
      buildPeer({
        state: {
          user: { id: 'usr_bob', name: 'Bob', avatarUrl: '/avatar-bob.png' },
          cursor: { position: { x: 0, y: 0 }, dragging: false },
          selection: { nodeIds: [], marquee: null },
        },
      }),
    ])

    expect(view.container.querySelector('img')?.getAttribute('src')).toBe('/avatar-bob.png')
  })

  test('falls back to the name initial without an avatar', async () => {
    renderOverlay([buildPeer()])

    expect(await screen.findByText('B')).toBeVisible()
  })

  test('falls back to the viewer session avatar for the self peer', () => {
    useUserStore().setAuth(
      makeSessionUser({
        public_id: 'usr_ada',
        avatar: '/api/v1/public/users/usr_ada/avatar?v=3',
      }),
    )
    const view = renderOverlay([
      buildPeer({
        clientId: 8,
        state: {
          user: { id: 'usr_ada', name: 'Ada' },
          cursor: { position: { x: 1, y: 1 }, dragging: false },
          selection: { nodeIds: [], marquee: null },
        },
      }),
    ])

    expect(view.container.querySelector('img')?.getAttribute('src')).toBe(
      '/api/v1/public/users/usr_ada/avatar?v=3&size=64',
    )
  })
})
