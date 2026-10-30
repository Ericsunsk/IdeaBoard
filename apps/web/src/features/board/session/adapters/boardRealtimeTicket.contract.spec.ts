// Presence ticket transport contract.
//
// MSW-level contract: exercises the real `api.request` transport against the
// AGREED realtime-ticket envelope, so backend drift surfaces here instead of
// in the canvas. The ticket is opaque: only its presence and shape are
// asserted, never its contents.
import { describe, expect, test } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import {
  BoardPayloadError,
  fetchWorkspaceBoardBundle,
  requestBoardRealtimeTicket,
} from './boardsResource'

const TICKET_PATH = '/api/v1/workspaces/ws-1/boards/board-1/realtime-ticket'
const BUNDLE_PATH = '/api/v1/workspaces/ws-1/boards/board-bundle-1/bundle'
const BUNDLE_TICKET_PATH = '/api/v1/workspaces/ws-1/boards/board-bundle-1/realtime-ticket'

describe('presence ticket transport', () => {
  test('mints the opaque ticket from the agreed envelope', async () => {
    useMswHandlers(
      http.post(TICKET_PATH, () =>
        HttpResponse.json({ data: { ticket: 'tkt_1', expires_at: '2026-10-19T08:01:00Z' } }),
      ),
    )

    await expect(requestBoardRealtimeTicket('ws-1', 'board-1')).resolves.toBe('tkt_1')
  })

  test('fetchWorkspaceBoardBundle caches the realtime ticket and avoids follow-up ticket request', async () => {
    let ticketCalls = 0
    const futureExpiry = new Date(Date.now() + 60_000).toISOString()
    useMswHandlers(
      http.get(BUNDLE_PATH, () =>
        HttpResponse.json({
          data: {
            meta: {
              id: 'board-bundle-1',
              workspace_id: 'ws-1',
              name: 'Bundle Board',
              revision: 1,
              schema_version: 1,
              creator_user_id: 'usr_owner',
              workspace_access: 'workspace_members',
              access_version: 1,
              access_state: 'active',
              relationship: 'member',
              capabilities: { can_read: true, can_write: true },
              created_at: '2026-10-19T08:00:00Z',
              updated_at: '2026-10-19T08:00:00Z',
            },
            document: {
              board_id: 'board-bundle-1',
              revision: 1,
              schema_version: 1,
              document: { schemaVersion: 1, nodes: [], edges: [] },
              updated_at: '2026-10-19T08:00:00Z',
            },
            realtime: {
              ticket: 'tkt_bundle_123',
              expires_at: futureExpiry,
            },
          },
        }),
      ),
      http.post(BUNDLE_TICKET_PATH, () => {
        ticketCalls++
        return HttpResponse.json({ data: { ticket: 'tkt_post', expires_at: futureExpiry } })
      }),
    )

    const bundle = await fetchWorkspaceBoardBundle('ws-1', 'board-bundle-1')
    expect(bundle.meta.id).toBe('board-bundle-1')
    expect(bundle.document.revision).toBe(1)

    // Requesting ticket immediately returns the bundled ticket without calling POST /realtime-ticket!
    const ticket = await requestBoardRealtimeTicket('ws-1', 'board-bundle-1')
    expect(ticket).toBe('tkt_bundle_123')
    expect(ticketCalls).toBe(0)
  })

  test('fails closed on transport errors', async () => {
    useMswHandlers(http.post(TICKET_PATH, () => HttpResponse.json({}, { status: 500 })))

    await expect(requestBoardRealtimeTicket('ws-1', 'board-1')).rejects.toThrow()
  })

  test('fails closed on malformed envelopes', async () => {
    useMswHandlers(http.post(TICKET_PATH, () => HttpResponse.json({ data: { ticket: '  ' } })))

    await expect(requestBoardRealtimeTicket('ws-1', 'board-1')).rejects.toBeInstanceOf(
      BoardPayloadError,
    )
  })

  test('fails closed without a workspace board', async () => {
    await expect(requestBoardRealtimeTicket('', 'board-1')).rejects.toBeInstanceOf(
      BoardPayloadError,
    )
    await expect(requestBoardRealtimeTicket('ws-1', '')).rejects.toBeInstanceOf(BoardPayloadError)
  })
})
