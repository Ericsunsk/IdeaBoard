// HTTP + WebSocket wiring for the realtime collaboration server.
//
// Flow per connection: parse ticket from /realtime?ticket= -> verifyTicket
// locally (signature + exp) -> admitToRoom against the Go internal endpoint ->
// only then attach to the room (room = bid) and speak the standard
// awareness + sync protocol. Rejections upgrade-then-close with a permanent
// 44xx code so unmodified y-websocket clients stop reconnecting (4400-4499 =
// permanent by the y-websocket close-code convention): 4401 ticket, 4403
// admit. Plain HTTP hits on /realtime get a 4xx JSON token instead.
//
// Document plane (ADR-0020): the room Doc (getMap('nodes') + getMap('edges'),
// keyed by id) converges SyncStep2/Update frames from ticket cap=write
// connections, broadcasts them to room peers for WS incremental convergence,
// and persists the opaque bytes to Go (POST /internal/realtime/y-updates,
// 4xx dropped, 5xx/transport retried with backoff). Every N applied updates
// the room module posts a stripped compaction snapshot
// (POST /internal/realtime/compactions) that Go validates with the same
// ValidateV1DocumentStrict rules and lands in board_documents +
// board_revisions. Snapshots still hydrate first over HTTP (GET board +
// previewscene, untouched); the non-collaborator If-Match + 409 path is
// untouched. Awareness stays standard forwarding.
//
// Kick model this phase: ticket window + reconnect denial only. There is no
// admit re-polling on live connections; phase-2 may add periodic admit
// re-checks that close living sockets with 4403 (see ADR-0019).

import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http'
import type { Duplex } from 'node:stream'
import { WebSocket, WebSocketServer } from 'ws'
import { admitToRoom } from '../admission/admit.js'
import { verifyTicket } from '../admission/ticket.js'
import { DurableRooms, type RoomAdmission, type RoomConnection } from './room.js'
import { YUpdatePersister } from './persist.js'

export type RealtimeServerConfig = {
  host: string
  port: number
  ticketSecret: string
  internalKey: string
  apiInternalURL: string
  /** Applied updates per room between compaction posts. Defaults to 50. */
  compactEveryUpdates?: number
  /** Persist retry budget. Defaults to 5. */
  persistMaxRetries?: number
  fetchFn?: typeof fetch
}

export type RealtimeMetrics = {
  startedAt: number
  admittedTotal: number
  rejectedTicketTotal: number
  rejectedAdmitTotal: number
}

const PING_INTERVAL_MS = 30_000
const CLOSE_TICKET = 4401
const CLOSE_ADMIT = 4403

function sendJson(res: ServerResponse, status: number, body: unknown): void {
  res.writeHead(status, { 'content-type': 'application/json' })
  res.end(JSON.stringify(body))
}

export function createRealtimeServer(config: RealtimeServerConfig): {
  httpServer: Server
  metrics: RealtimeMetrics
  close: () => Promise<void>
} {
  const metrics: RealtimeMetrics = {
    startedAt: Date.now(),
    admittedTotal: 0,
    rejectedTicketTotal: 0,
    rejectedAdmitTotal: 0,
  }
  const wss = new WebSocketServer({ noServer: true })
  const liveSockets = new Set<WebSocket>()
  const persister = new YUpdatePersister({
    apiInternalURL: config.apiInternalURL,
    internalKey: config.internalKey,
    maxRetries: config.persistMaxRetries,
    fetchFn: config.fetchFn,
  })
  const rooms = new DurableRooms({
    persistence: persister,
    compactEveryUpdates: config.compactEveryUpdates,
  })

  const httpServer = createServer((req, res) => {
    const url = new URL(req.url ?? '/', 'http://localhost')
    if (req.method === 'GET' && url.pathname === '/health') {
      sendJson(res, 200, {
        code: 0,
        msg: 'success',
        data: {
          ok: true,
          ...rooms.metrics,
          persistPending: persister.pendingCount,
          persistedTotal: persister.persistedTotal,
          persistDroppedTotal: persister.droppedTotal,
          persistFailedTotal: persister.failedTotal,
          persistRetriedTotal: persister.retriedTotal,
          compactedTotal: persister.compactedTotal,
          compactFailedTotal: persister.compactFailedTotal,
          admittedTotal: metrics.admittedTotal,
          rejectedTicketTotal: metrics.rejectedTicketTotal,
          rejectedAdmitTotal: metrics.rejectedAdmitTotal,
          uptimeMs: Date.now() - metrics.startedAt,
        },
      })
      return
    }
    if (url.pathname === '/realtime' || url.pathname.startsWith('/realtime/')) {
      sendJson(res, 426, { code: 426, msg: 'websocket upgrade required' })
      return
    }
    sendJson(res, 404, { code: 404, msg: 'not found' })
  })

  httpServer.on('upgrade', (req, socket, head) => {
    void handleUpgrade(req, socket as Duplex, head as Buffer).catch(() => {
      try {
        socket.destroy()
      } catch {
        // Socket already gone; nothing to clean up.
      }
    })
  })

  async function handleUpgrade(req: IncomingMessage, socket: Duplex, head: Buffer): Promise<void> {
    const url = new URL(req.url ?? '/', 'ws://localhost')
    if (url.pathname !== '/realtime') {
      socket.write('HTTP/1.1 404 Not Found\r\nConnection: close\r\n\r\n')
      socket.destroy()
      return
    }
    const ticket = url.searchParams.get('ticket') ?? ''
    const verified = verifyTicket(ticket, config.ticketSecret)
    if (!verified.ok) {
      metrics.rejectedTicketTotal += 1
      rejectUpgrade(req, socket, head, CLOSE_TICKET, verified.reason)
      return
    }
    const admit = await admitToRoom({
      apiInternalURL: config.apiInternalURL,
      internalKey: config.internalKey,
      ticket,
      fetchFn: config.fetchFn,
    })
    if (!admit.admitted) {
      metrics.rejectedAdmitTotal += 1
      rejectUpgrade(req, socket, head, CLOSE_ADMIT, 'admit denied')
      return
    }
    metrics.admittedTotal += 1
    let admission: RoomAdmission
    try {
      admission = await rooms.prepare(verified.payload.bid, ticket)
    } catch {
      rejectUpgrade(req, socket, head, CLOSE_ADMIT, 'document recovery unavailable')
      return
    }
    const identity = { uid: verified.payload.uid, canWrite: verified.payload.cap === 'write' }
    try {
      wss.handleUpgrade(req, socket, head, (ws) => {
        attachPresenceConnection(ws as WebSocket, admission.attach(ws, identity))
      })
    } finally {
      admission.cancel()
    }
  }

  // Accept the handshake so the client learns the permanent close code and
  // reason instead of an opaque upgrade failure, then close immediately.
  function rejectUpgrade(
    req: IncomingMessage,
    socket: Duplex,
    head: Buffer,
    code: number,
    reason: string,
  ): void {
    wss.handleUpgrade(req, socket, head, (ws) => {
      ;(ws as WebSocket).close(code, reason)
    })
  }

  function attachPresenceConnection(ws: WebSocket, connection: RoomConnection): void {
    liveSockets.add(ws)
    let alive = true
    const pingTimer = setInterval(() => {
      if (!alive) {
        cleanup()
        try {
          ws.terminate()
        } catch {
          // Already closed; cleanup covers the room bookkeeping.
        }
        return
      }
      alive = false
      try {
        ws.ping()
      } catch {
        cleanup()
      }
    }, PING_INTERVAL_MS)
    pingTimer.unref()

    ws.on('pong', () => {
      alive = true
    })
    ws.on('message', (data) => {
      // RawData is Buffer | ArrayBuffer | Buffer[]; malformed frames drop
      // closed (handleMessage never throws, this guard is only for framing).
      let bytes: Uint8Array | null = null
      try {
        if (data instanceof Uint8Array) bytes = data
        else if (data instanceof ArrayBuffer) bytes = new Uint8Array(data)
        else if (Array.isArray(data)) bytes = new Uint8Array(Buffer.concat(data))
      } catch {
        bytes = null
      }
      if (bytes === null) return
      const reply = connection.receive(bytes)
      if (reply !== null && ws.readyState === WebSocket.OPEN) {
        try {
          ws.send(reply)
        } catch {
          // Send failure surfaces on close; nothing else to do here.
        }
      }
    })
    const cleanup = () => {
      clearInterval(pingTimer)
      liveSockets.delete(ws)
      connection.close()
    }
    ws.on('close', cleanup)
    ws.on('error', () => {
      // Errors always end in close; bookkeeping happens there.
    })

    for (const message of connection.initialMessages()) {
      try {
        ws.send(message)
      } catch {
        cleanup()
        try {
          ws.terminate()
        } catch {
          // Terminate already attempted; cleanup is idempotent.
        }
        return
      }
    }
  }

  async function close(): Promise<void> {
    for (const ws of liveSockets) {
      try {
        ws.close(1012, 'server shutdown')
      } catch {
        // Best effort during shutdown.
      }
    }
    await rooms.close()
    wss.close()
    await new Promise<void>((resolve, reject) => {
      httpServer.close((error) => (error ? reject(error) : resolve()))
    })
  }

  return { httpServer, metrics, close }
}
