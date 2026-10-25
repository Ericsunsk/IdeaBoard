import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import Fastify from 'fastify'
import {
  MailConfigService,
  type MailConfigRepository,
  type MailSendInput,
  type StoredMailConfig,
} from './mailConfig.js'
import { registerWorkspaceInvitationRoutes } from './workspaceInvitationRoutes.js'

const internalKey = 'workspace-invitation-internal-key'
const payload = {
  recipient: 'member@example.com',
  invite_id: 'wsi_test',
  workspace_name: 'Studio <Design>',
  inviter_name: 'Alex & Sam',
  role: 'member',
  token: 'a'.repeat(64),
}

describe('Workspace invitation delivery', () => {
  it('requires the internal key and rejects arbitrary mail content or redirects', async (t) => {
    const { app, deliveries } = createApp()
    t.after(() => app.close())
    const unauthorized = await app.inject({
      method: 'POST',
      url: '/internal/mail/workspace-invitation',
      payload,
    })
    assert.equal(unauthorized.statusCode, 401)
    for (const extra of [{ html: '<script>secret</script>' }, { url: 'https://evil.example' }]) {
      const response = await app.inject({
        method: 'POST',
        url: '/internal/mail/workspace-invitation',
        headers: { 'x-internal-key': internalKey },
        payload: { ...payload, ...extra },
      })
      assert.equal(response.statusCode, 400)
    }
    assert.equal(deliveries.length, 0)
  })

  it('sends escaped Workspace content with a trusted link and stable provider delivery key', async (t) => {
    const { app, deliveries } = createApp()
    t.after(() => app.close())
    const response = await app.inject({
      method: 'POST',
      url: '/internal/mail/workspace-invitation',
      headers: { 'x-internal-key': internalKey },
      payload,
    })
    assert.equal(response.statusCode, 200)
    assert.deepEqual(response.json(), { code: 0, msg: 'success', data: { ok: true } })
    assert.equal(response.headers['cache-control'], 'no-store')
    assert.doesNotMatch(response.body, new RegExp(payload.token))
    assert.equal(deliveries.length, 1)
    const delivery = deliveries[0]!
    assert.equal(delivery.to, payload.recipient)
    assert.equal(delivery.idempotencyKey, 'workspace-invite:wsi_test')
    assert.match(delivery.content.html, /Studio &lt;Design&gt;/)
    assert.match(delivery.content.html, /Alex &amp; Sam/)
    assert.match(delivery.content.text, /workspace as member/)
    assert.match(
      delivery.content.text,
      new RegExp(`https://app.example.com/invites/workspace\\?token=${payload.token}`),
    )
    assert.doesNotMatch(delivery.content.text, /board as/)
  })

  it('rejects invalid invitation facts before attempting delivery', async (t) => {
    const { app, deliveries } = createApp()
    t.after(() => app.close())
    for (const invalid of [
      { recipient: 'not-email' },
      { token: 'not-a-token' },
      { invite_id: 'osi_other' },
      { role: 'owner' },
      { workspace_name: '' },
      { inviter_name: 'name\nBcc: attacker@example.com' },
    ]) {
      const response = await app.inject({
        method: 'POST',
        url: '/internal/mail/workspace-invitation',
        headers: { 'x-internal-key': internalKey },
        payload: { ...payload, ...invalid },
      })
      assert.equal(response.statusCode, 400)
    }
    assert.equal(deliveries.length, 0)
  })

  it('does not report delivery success when mail is unready or the public origin is unavailable', async (t) => {
    for (const options of [{ readiness: 'not_tested' as const }, { origin: '' }]) {
      const { app, deliveries } = createApp(options)
      t.after(() => app.close())
      const response = await app.inject({
        method: 'POST',
        url: '/internal/mail/workspace-invitation',
        headers: { 'x-internal-key': internalKey },
        payload,
      })
      assert.equal(response.statusCode, 503)
      assert.equal(deliveries.length, 0)
    }
  })

  it('propagates provider failure without exposing recipient, token, or provider details', async (t) => {
    const { app } = createApp({ deliveryError: new Error(`provider leaked ${payload.token}`) })
    t.after(() => app.close())
    const response = await app.inject({
      method: 'POST',
      url: '/internal/mail/workspace-invitation',
      headers: { 'x-internal-key': internalKey },
      payload,
    })
    assert.equal(response.statusCode, 502)
    assert.equal(response.json().msg, 'workspace invitation delivery failed')
    assert.doesNotMatch(response.body, new RegExp(payload.token))
  })
})

function createApp(
  options: {
    readiness?: StoredMailConfig['readiness']
    origin?: string
    deliveryError?: Error
  } = {},
) {
  const deliveries: MailSendInput[] = []
  const repository: MailConfigRepository = {
    get: async () => ({
      encryptedApiKey: 'ciphertext',
      enabled: true,
      fromName: 'IdeaBoard',
      fromEmail: 'mail@example.com',
      replyTo: '',
      configVersion: 1,
      readiness: options.readiness ?? 'ready',
      updatedAt: new Date(),
      lastTestedAt: new Date(),
      lastTestErrorCode: null,
    }),
    put: async () => {
      throw new Error('unexpected configuration write')
    },
    markTest: async () => {
      throw new Error('unexpected readiness mutation')
    },
    clear: async () => {
      throw new Error('unexpected configuration delete')
    },
  }
  const service = new MailConfigService(
    repository,
    {
      send: async (input) => {
        if (options.deliveryError) throw options.deliveryError
        deliveries.push(input)
      },
    },
    { encrypt: (value) => value, decrypt: () => 'test-provider-key' },
    options.origin ?? 'https://app.example.com',
  )
  const app = Fastify()
  registerWorkspaceInvitationRoutes(app, { internalKey, service })
  return { app, deliveries }
}
