import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
import { pathToFileURL } from 'node:url'
import { join } from 'node:path'
import { test } from 'node:test'
import type { Page, Route } from 'playwright'
import { installAnonymousSmokeApi, type SmokeConfig } from './smokeSupport'

test('stopping a directly spawned Vite releases its port and lets the caller exit', async () => {
  const helperUrl = pathToFileURL(join(import.meta.dirname, 'smokeSupport.ts')).href
  const script = `
    import assert from 'node:assert/strict';
    import { resolveSmokePort, spawnSmokeDevServer, waitForSmokeDevReady, stopSmokeDevServer }
      from ${JSON.stringify(helperUrl)};
    const host = '127.0.0.1';
    const port = await resolveSmokePort(host, 5273);
    const { child } = spawnSmokeDevServer(host, port);
    try {
      await waitForSmokeDevReady('http://' + host + ':' + port, child);
    } finally {
      await stopSmokeDevServer(child);
    }
    assert.ok(child.exitCode !== null || child.signalCode !== null);
    assert.throws(() => process.kill(child.pid, 0), { code: 'ESRCH' });
    assert.equal(await resolveSmokePort(host, port), port);
  `
  // No process.exit(): leaked servers, log pipes or timers must fail the timeout.
  await promisify(execFile)(
    process.execPath,
    ['--import', 'tsx', '--input-type=module', '-e', script],
    {
      timeout: 15_000,
      killSignal: 'SIGKILL',
    },
  )
})

const config: SmokeConfig = {
  host: '127.0.0.1',
  port: 5173,
  baseUrlOverride: null,
  apiBaseUrl: null,
  verbose: false,
}

test('anonymous fixtures restrict origin, method, and endpoint without touching reused servers', async () => {
  let handler: ((route: Route) => Promise<void>) | undefined
  const page = {
    route: async (matches: (url: URL) => boolean, callback: typeof handler) => {
      assert.equal(matches(new URL('http://127.0.0.1:5173/api/v1/user')), true)
      assert.equal(matches(new URL('http://127.0.0.1:5173/src/api/client.ts')), false)
      handler = callback
    },
  } as unknown as Page
  const failures: string[] = []
  await installAnonymousSmokeApi(page, config, 'http://127.0.0.1:5173', failures)
  assert.ok(handler)
  async function dispatch(path: string, method = 'GET', origin = 'http://127.0.0.1:5173') {
    let reply: Parameters<Route['fulfill']>[0] | undefined
    let aborted = false
    await handler!({
      request: () => ({ url: () => `${origin}${path}`, method: () => method }),
      fulfill: async (response: typeof reply) => {
        reply = response
      },
      abort: async () => {
        aborted = true
      },
    } as unknown as Route)
    return { reply, aborted }
  }
  const session = await dispatch('/api/auth/get-session')
  assert.equal(session.reply?.status, 200)
  assert.equal(session.reply?.body, 'null')
  assert.equal((await dispatch('/api/v1/user')).reply?.status, 401)
  const search = await dispatch('/api/v1/search?q=smoke&limit=24')
  assert.deepEqual(JSON.parse(search.reply?.body as string), {
    data: { query: 'smoke', results: [], has_more: false },
  })
  for (const [path, method, origin] of [
    ['/api/v1/settings/branding', 'POST', 'http://127.0.0.1:5173'],
    ['/api/unexpected', 'GET', 'http://127.0.0.1:5173'],
    ['/api/v1/settings/branding', 'GET', 'https://external.example'],
  ]) {
    const result = await dispatch(path!, method!, origin!)
    assert.equal(result.aborted, true)
    assert.equal(result.reply, undefined)
  }
  assert.equal(failures.length, 3)
  handler = undefined
  await installAnonymousSmokeApi(
    page,
    { ...config, baseUrlOverride: 'http://127.0.0.1:6000' },
    'http://127.0.0.1:6000',
    failures,
  )
  assert.equal(handler, undefined)
})
