// Presence ticket transport contract.
//
// MSW-level contract: exercises the real `api.request` transport against the
// AGREED realtime-ticket envelope, so backend drift surfaces here instead of
// in the canvas. The ticket is opaque: only its presence and shape are
// asserted, never its contents.
import { describe, expect, test } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../../test/msw'
import { PresenceTicketError, requestPresenceTicket } from './ticket'

const TICKET_PATH = '/api/v1/workspaces/ws-1/boards/board-1/realtime-ticket'

describe('presence ticket transport', () => {
  test('mints the opaque ticket from the agreed envelope', async () => {
    useMswHandlers(
      http.post(TICKET_PATH, () =>
        HttpResponse.json({ data: { ticket: 'tkt_1', expires_at: '2026-10-19T08:01:00Z' } }),
      ),
    )

    await expect(requestPresenceTicket('ws-1', 'board-1')).resolves.toBe('tkt_1')
  })

  test('fails closed on transport errors', async () => {
    useMswHandlers(http.post(TICKET_PATH, () => HttpResponse.json({}, { status: 500 })))

    await expect(requestPresenceTicket('ws-1', 'board-1')).rejects.toBeInstanceOf(
      PresenceTicketError,
    )
  })

  test('fails closed on malformed envelopes', async () => {
    useMswHandlers(http.post(TICKET_PATH, () => HttpResponse.json({ data: { ticket: '  ' } })))

    await expect(requestPresenceTicket('ws-1', 'board-1')).rejects.toBeInstanceOf(
      PresenceTicketError,
    )
  })

  test('fails closed without a workspace board', async () => {
    await expect(requestPresenceTicket('', 'board-1')).rejects.toBeInstanceOf(PresenceTicketError)
    await expect(requestPresenceTicket('ws-1', '')).rejects.toBeInstanceOf(PresenceTicketError)
  })
})
