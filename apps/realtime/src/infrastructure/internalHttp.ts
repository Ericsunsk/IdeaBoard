// Internal Go calls must not use global fetch: the Fetch spec's bad-port
// list blocks port 6000 (X11), so fetch to the Go API fails with
// "bad port" while curl/Go succeed. node:http has no such filter.
import http from 'node:http'
import https from 'node:https'

export type InternalFetchInit = {
  method?: string
  headers?: Record<string, string>
  body?: string
  signal?: AbortSignal | null
}

export type InternalFetchResponse = {
  ok: boolean
  status: number
  headers: { get(name: string): string | null }
  text(): Promise<string>
}

export async function internalFetch(
  input: string,
  init: InternalFetchInit = {},
): Promise<InternalFetchResponse> {
  const url = new URL(input)
  const lib = url.protocol === 'https:' ? https : http
  const headers: Record<string, string> = { ...(init.headers ?? {}) }
  const payload = init.body === undefined ? undefined : Buffer.from(init.body)
  if (payload && !headers['content-length'] && !headers['Content-Length']) {
    headers['content-length'] = String(payload.length)
  }
  return new Promise((resolve, reject) => {
    if (init.signal?.aborted) {
      reject(new Error('aborted'))
      return
    }
    const req = lib.request(
      url,
      { method: init.method ?? 'GET', headers, signal: init.signal ?? undefined },
      (res) => {
        const chunks: Buffer[] = []
        res.on('error', reject)
        res.on('aborted', () => reject(new Error('internal response aborted')))
        res.on('data', (c: Buffer) => chunks.push(c))
        res.on('end', () => {
          const text = Buffer.concat(chunks).toString('utf8')
          resolve({
            ok: (res.statusCode ?? 500) >= 200 && (res.statusCode ?? 500) < 300,
            status: res.statusCode ?? 0,
            headers: {
              get: (name: string) => {
                const v = res.headers[name.toLowerCase()]
                return Array.isArray(v) ? v.join(', ') : (v ?? null)
              },
            },
            text: async () => text,
          })
        })
      },
    )
    req.on('error', reject)
    if (payload) req.write(payload)
    req.end()
  })
}
