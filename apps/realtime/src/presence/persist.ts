// Y update persistence fan-out (ADR-0020). The room Doc applies client
// SyncStep2/Update frames and broadcasts them to peers; this persister
// forwards the opaque update bytes to the Go internal surface
// (POST /internal/realtime/y-updates, X-Internal-Key) so they land in
// board_y_updates. Compaction snapshots go to
// POST /internal/realtime/compactions via compactNow.
//
// Retry contract: 4xx outcomes are permanent (drop, never retry — the Go
// side fails those closed on validation/capability); 5xx and transport
// failures retry with bounded exponential backoff. Awareness stays standard
// forwarding and never touches this path.

export type PersistOptions = {
  apiInternalURL: string
  internalKey: string
  maxRetries?: number
  baseDelayMs?: number
  maxDelayMs?: number
  fetchFn?: typeof fetch
}

type PostResult = {
  ok: boolean
  status: number
}

const DEFAULT_MAX_RETRIES = 5
const DEFAULT_BASE_DELAY_MS = 200
const DEFAULT_MAX_DELAY_MS = 5_000
const MAX_HYDRATION_BYTES = 8 << 20
const MAX_BODY_BYTES = 2 * 1024 * 1024

function toBase64(update: Uint8Array): string {
  return Buffer.from(update.buffer, update.byteOffset, update.byteLength).toString('base64')
}

function isRetryableStatus(status: number): boolean {
  return status === 0 || status >= 500
}

function backoffDelay(attempt: number, baseDelayMs: number, maxDelayMs: number): number {
  return Math.min(baseDelayMs * 2 ** attempt, maxDelayMs)
}

export class YUpdatePersister {
  persistedTotal = 0
  droppedTotal = 0
  failedTotal = 0
  retriedTotal = 0
  pendingCount = 0
  compactedTotal = 0
  compactFailedTotal = 0

  private readonly apiInternalURL: string
  private readonly internalKey: string
  private readonly maxRetries: number
  private readonly baseDelayMs: number
  private readonly maxDelayMs: number
  private readonly fetchFn: typeof fetch
  private readonly timers = new Map<ReturnType<typeof setTimeout>, () => void>()
  private closed = false

  constructor(options: PersistOptions) {
    this.apiInternalURL = options.apiInternalURL.replace(/\/$/, '')
    this.internalKey = options.internalKey
    this.maxRetries = options.maxRetries ?? DEFAULT_MAX_RETRIES
    this.baseDelayMs = options.baseDelayMs ?? DEFAULT_BASE_DELAY_MS
    this.maxDelayMs = options.maxDelayMs ?? DEFAULT_MAX_DELAY_MS
    this.fetchFn = options.fetchFn ?? fetch
  }

  /** Bounded recovery transport; the room module owns replay/admission ordering. */
  async load(ticket: string): Promise<{ snapshot: unknown; updates: Uint8Array[] }> {
    if (this.closed) throw new Error('persistence is closed')
    const endpoint = new URL('/internal/realtime/document', `${this.apiInternalURL}/`)
    endpoint.searchParams.set('ticket', ticket)
    const response = await this.fetchFn(endpoint.toString(), {
      method: 'GET',
      headers: { 'x-internal-key': this.internalKey },
    })
    if (!response.ok) throw new Error(`document hydration failed: ${response.status}`)
    if (Number(response.headers.get('content-length') ?? 0) > MAX_HYDRATION_BYTES) {
      throw new Error('document hydration exceeds size limit')
    }
    const body = await response.text()
    if (Buffer.byteLength(body) > MAX_HYDRATION_BYTES)
      throw new Error('document hydration exceeds size limit')
    const envelope = JSON.parse(body) as {
      code?: number
      data?: { snapshot?: unknown; updates?: unknown }
    }
    if (envelope.code !== 0 || !envelope.data || !Array.isArray(envelope.data.updates)) {
      throw new Error('invalid document hydration response')
    }
    const updates = envelope.data.updates.map((encoded) => {
      if (typeof encoded !== 'string') throw new Error('invalid persisted update')
      return new Uint8Array(Buffer.from(encoded, 'base64'))
    })
    return { snapshot: envelope.data.snapshot, updates }
  }

  /** Append one update; true means Go acknowledged the durable write. */
  async append(bid: string, update: Uint8Array, authorUid: string): Promise<boolean> {
    if (this.closed) return false
    this.pendingCount += 1
    try {
      const result = await this.postWithRetry('/internal/realtime/y-updates', {
        board_id: bid,
        update: toBase64(update),
        author_uid: authorUid,
      })
      if (result.ok) this.persistedTotal += 1
      else if (isRetryableStatus(result.status)) this.failedTotal += 1
      else this.droppedTotal += 1
      return result.ok
    } finally {
      this.pendingCount -= 1
    }
  }

  /** Post one compaction snapshot. Resolves true only on Go acknowledgement. */
  async compactNow(bid: string, snapshot: string, authorUid: string): Promise<boolean> {
    if (this.closed) return false
    const result = await this.postWithRetry('/internal/realtime/compactions', {
      board_id: bid,
      snapshot: JSON.parse(snapshot) as unknown,
      author_uid: authorUid,
    })
    if (result.ok) this.compactedTotal += 1
    else this.compactFailedTotal += 1
    return result.ok
  }

  close(): void {
    this.closed = true
    for (const [timer, resolve] of this.timers) {
      clearTimeout(timer)
      resolve()
    }
    this.timers.clear()
  }

  private async postWithRetry(path: string, body: Record<string, unknown>): Promise<PostResult> {
    const encoded = JSON.stringify(body)
    if (Buffer.byteLength(encoded) > MAX_BODY_BYTES) return { ok: false, status: 413 }
    for (let attempt = 0; ; attempt += 1) {
      if (this.closed) return { ok: false, status: 0 }
      let result: PostResult
      try {
        const response = await this.fetchFn(`${this.apiInternalURL}${path}`, {
          method: 'POST',
          headers: {
            'content-type': 'application/json',
            'x-internal-key': this.internalKey,
          },
          body: encoded,
        })
        // Drain the body so the socket can be reused.
        try {
          await response.text()
        } catch {
          // Body drain is best effort.
        }
        result = { ok: response.ok, status: response.status }
      } catch {
        result = { ok: false, status: 0 }
      }
      if (result.ok || !isRetryableStatus(result.status) || attempt >= this.maxRetries) {
        return result
      }
      this.retriedTotal += 1
      await this.sleep(backoffDelay(attempt, this.baseDelayMs, this.maxDelayMs))
    }
  }

  private sleep(ms: number): Promise<void> {
    if (this.closed) return Promise.resolve()
    return new Promise((resolve) => {
      const timer = setTimeout(() => {
        this.timers.delete(timer)
        resolve()
      }, ms)
      this.timers.set(timer, resolve)
    })
  }
}
