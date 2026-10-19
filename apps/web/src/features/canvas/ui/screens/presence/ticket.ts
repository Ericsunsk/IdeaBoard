// Canvas presence ticket resource.
//
// Mints the short-lived board realtime ticket that authorizes one
// y-websocket room: `POST /api/v1/workspaces/{ws}/boards/{board}/realtime-ticket`
// → `{ data: { ticket, expires_at } }` (60-second HMAC bearer; viewers may
// mint, no Idempotency-Key). This module is the adapter boundary for that
// call (raw `api.request`: components never call it directly). The ticket
// response is parsed fail-closed; the ticket itself is opaque and only ever
// travels as the `/realtime` `ticket` query param, never inside document
// payloads. Every (re)connect re-signs, so expiry is handled by rotation.

import { api } from '../../../../../api/client'

export class PresenceTicketError extends Error {
  constructor(message = 'Presence ticket request failed') {
    super(message)
    this.name = 'PresenceTicketError'
  }
}

function presenceTicketSuffix(workspaceId: string, boardId: string): string {
  const workspace = encodeURIComponent(workspaceId)
  const board = encodeURIComponent(boardId)
  return `/v1/workspaces/${workspace}/boards/${board}/realtime-ticket`
}

function readTicket(envelope: unknown): string | null {
  if (!envelope || typeof envelope !== 'object') return null
  const data = (envelope as Record<string, unknown>).data
  if (!data || typeof data !== 'object') return null
  const ticket = (data as Record<string, unknown>).ticket
  if (typeof ticket !== 'string' || !ticket.trim()) return null
  return ticket.trim()
}

export async function requestPresenceTicket(
  workspaceId: string,
  boardId: string,
  options?: { signal?: AbortSignal | null | undefined },
): Promise<string> {
  const workspace = workspaceId.trim()
  const board = boardId.trim()
  if (!workspace || !board) throw new PresenceTicketError('Presence ticket needs a workspace board')
  let envelope: unknown
  try {
    envelope = await api.request<unknown>(presenceTicketSuffix(workspace, board), {
      method: 'POST',
      ...(options?.signal === undefined ? {} : { signal: options.signal }),
    })
  } catch {
    throw new PresenceTicketError('Presence ticket request failed')
  }
  const ticket = readTicket(envelope)
  if (!ticket) throw new PresenceTicketError('Presence ticket response is invalid')
  return ticket
}
