import assert from 'node:assert/strict'
import { randomBytes } from 'node:crypto'
import { describe, it } from 'node:test'
import { createSecretCipher } from '../infrastructure/secretCipher.js'
import {
  MailConfigError,
  MailConfigService,
  type MailConfigRepository,
  type MailReadiness,
  type MailSendInput,
  type StoredMailConfig,
} from './mailConfig.js'
import { MailDeliveryError } from './mailConfigAdapters.js'

const secretCipher = createSecretCipher(randomBytes(32).toString('base64'))

describe('MailConfigService', () => {
  it('encrypts a new API key, never returns it, and resets readiness after configuration changes', async () => {
    const repository = createFakeRepository()
    const service = new MailConfigService(repository.port, { send: async () => {} }, secretCipher)

    const summary = await service.update({
      apiKey: 're_super_secret',
      fromName: ' IdeaBoard ',
      fromEmail: ' Auth@Example.com ',
      replyTo: ' Support@Example.com ',
      actorId: 7,
    })

    assert.equal(summary.readiness, 'not_tested')
    assert.equal(summary.apiKeyConfigured, true)
    assert.equal(summary.fromEmail, 'auth@example.com')
    assert.doesNotMatch(JSON.stringify(summary), /re_super_secret/)
    assert.ok(repository.value)
    assert.notEqual(repository.value.encryptedApiKey, 're_super_secret')
    assert.doesNotMatch(JSON.stringify(repository.audits), /re_super_secret/)
  })

  it('retains an existing API key when the update omits it', async () => {
    const repository = createFakeRepository()
    const service = new MailConfigService(repository.port, { send: async () => {} }, secretCipher)
    await service.update({
      apiKey: 're_initial',
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 1,
    })
    const encrypted = repository.value?.encryptedApiKey

    await service.update({
      fromName: 'IdeaBoard Mail',
      fromEmail: 'auth@example.com',
      actorId: 2,
    })

    assert.equal(repository.value?.encryptedApiKey, encrypted)
    assert.equal(repository.value?.readiness, 'not_tested')
  })

  it('keeps Ready for an identical save and invalidates it only when sender material changes', async () => {
    const repository = createFakeRepository()
    const service = new MailConfigService(repository.port, { send: async () => {} }, secretCipher)
    await service.update({
      apiKey: 're_initial',
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 1,
    })
    await service.sendTest({ recipient: 'admin@example.com', actorId: 1 })

    const unchanged = await service.update({
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 2,
    })
    const changed = await service.update({
      fromName: 'IdeaBoard Mail',
      fromEmail: 'auth@example.com',
      actorId: 2,
    })

    assert.equal(unchanged.readiness, 'ready')
    assert.equal(changed.readiness, 'not_tested')
  })

  it('binds readiness to a successful test with the current encrypted configuration', async () => {
    const repository = createFakeRepository()
    const sends: Array<{ input: MailSendInput; apiKey: string }> = []
    const service = new MailConfigService(
      repository.port,
      {
        send: async (input, apiKey) => {
          sends.push({ input, apiKey })
        },
      },
      secretCipher,
    )
    await service.update({
      apiKey: 're_runtime',
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 1,
    })

    await service.sendTest({ recipient: ' Admin@Example.com ', actorId: 2 })

    assert.equal(repository.value?.readiness, 'ready')
    assert.equal(sends[0]?.apiKey, 're_runtime')
    assert.equal(sends[0]?.input.to, 'admin@example.com')
    assert.ok(sends[0]?.input.content.text)
    assert.deepEqual(repository.audits.at(-1), {
      action: 'test_mail_success',
      actorId: 2,
      metadata: { errorCode: null },
    })
  })

  it('records only a normalized provider error and marks the configuration unavailable', async () => {
    const repository = createFakeRepository()
    const service = new MailConfigService(
      repository.port,
      {
        send: async () => {
          throw new Error('api-key=re_do_not_log provider raw payload')
        },
      },
      secretCipher,
    )
    await service.update({
      apiKey: 're_runtime',
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 1,
    })

    await assert.rejects(
      () => service.sendTest({ recipient: 'admin@example.com', actorId: 2 }),
      (error) => error instanceof MailConfigError && error.code === 'provider_error',
    )

    assert.equal(repository.value?.readiness, 'unavailable')
    assert.equal(repository.value?.lastTestErrorCode, 'provider_unavailable')
    assert.doesNotMatch(JSON.stringify(repository.audits), /re_do_not_log|raw payload/)
  })

  it('fails closed until the current configuration has passed a real test', async () => {
    const repository = createFakeRepository()
    const service = new MailConfigService(repository.port, { send: async () => {} }, secretCipher)
    await service.update({
      apiKey: 're_runtime',
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 1,
    })

    await assert.rejects(
      () =>
        service.send({
          to: 'user@example.com',
          content: { subject: 'Test', html: '<p>Test</p>', text: 'Test' },
        }),
      (error) => error instanceof MailConfigError && error.code === 'unavailable',
    )
  })

  it('does not mark a newer configuration Ready when an older test finishes late', async () => {
    const repository = createFakeRepository()
    let service: MailConfigService
    service = new MailConfigService(
      repository.port,
      {
        send: async () => {
          await service.update({
            fromName: 'IdeaBoard v2',
            fromEmail: 'auth@example.com',
            actorId: 3,
          })
        },
      },
      secretCipher,
    )
    await service.update({
      apiKey: 're_runtime',
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 1,
    })

    await assert.rejects(
      () => service.sendTest({ recipient: 'admin@example.com', actorId: 2 }),
      (error) => error instanceof MailConfigError && error.code === 'config_changed',
    )

    assert.equal(repository.value?.fromName, 'IdeaBoard v2')
    assert.equal(repository.value?.readiness, 'not_tested')
  })

  it('retries a concurrent update against fresh state without restoring an older API key', async () => {
    const repository = createFakeRepository()
    let testedAPIKey = ''
    const service = new MailConfigService(
      repository.port,
      {
        send: async (_input, apiKey) => {
          testedAPIKey = apiKey
        },
      },
      secretCipher,
    )
    await service.update({
      apiKey: 're_initial',
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 1,
    })

    let injected = false
    repository.beforePut = async () => {
      if (injected) return
      injected = true
      repository.beforePut = undefined
      await service.update({
        apiKey: 're_rotated',
        fromName: 'IdeaBoard',
        fromEmail: 'auth@example.com',
        actorId: 2,
      })
    }

    await service.update({
      fromName: 'IdeaBoard Mail',
      fromEmail: 'auth@example.com',
      actorId: 3,
    })
    await service.sendTest({ recipient: 'admin@example.com', actorId: 3 })

    assert.equal(testedAPIKey, 're_rotated')
    assert.equal(repository.value?.fromName, 'IdeaBoard Mail')
  })

  it('does not accept a stale test after clear and recreate', async () => {
    const repository = createFakeRepository()
    let service: MailConfigService
    service = new MailConfigService(
      repository.port,
      {
        send: async () => {
          await service.clear(2)
          await service.update({
            apiKey: 're_recreated',
            fromName: 'IdeaBoard v2',
            fromEmail: 'auth@example.com',
            actorId: 3,
          })
        },
      },
      secretCipher,
    )
    await service.update({
      apiKey: 're_initial',
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 1,
    })
    const originalVersion = repository.value?.configVersion

    await assert.rejects(
      () => service.sendTest({ recipient: 'admin@example.com', actorId: 4 }),
      (error) => error instanceof MailConfigError && error.code === 'config_changed',
    )

    assert.ok((repository.value?.configVersion ?? 0) > (originalVersion ?? 0))
    assert.equal(repository.value?.readiness, 'not_tested')
  })

  it('reports readiness persistence failure without marking a successful delivery unavailable', async () => {
    const repository = createFakeRepository()
    const service = new MailConfigService(repository.port, { send: async () => {} }, secretCipher)
    await service.update({
      apiKey: 're_runtime',
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 1,
    })
    repository.markTestError = new Error('database unavailable')

    await assert.rejects(
      () => service.sendTest({ recipient: 'admin@example.com', actorId: 2 }),
      (error) => error instanceof MailConfigError && error.code === 'repository_error',
    )

    assert.equal(repository.markTestCalls, 1)
    assert.equal(repository.value?.readiness, 'not_tested')
  })

  it('preserves both delivery and readiness persistence failures', async () => {
    const repository = createFakeRepository()
    const deliveryError = new MailDeliveryError('non_2xx', 'Resend rejected email')
    const service = new MailConfigService(
      repository.port,
      {
        send: async () => {
          throw deliveryError
        },
      },
      secretCipher,
    )
    await service.update({
      apiKey: 're_runtime',
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 1,
    })
    const persistenceError = new Error('database unavailable')
    repository.markTestError = persistenceError

    await assert.rejects(
      () => service.sendTest({ recipient: 'admin@example.com', actorId: 2 }),
      (error) =>
        error instanceof AggregateError &&
        error.errors.includes(deliveryError) &&
        error.errors.includes(persistenceError),
    )
  })

  it('sanitizes transport failures from transactional sends', async () => {
    const repository = createFakeRepository()
    let sendCount = 0
    const service = new MailConfigService(
      repository.port,
      {
        send: async () => {
          sendCount += 1
          if (sendCount === 1) return
          throw new Error('api-key=re_secret raw provider payload')
        },
      },
      secretCipher,
    )
    await service.update({
      apiKey: 're_runtime',
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 1,
    })
    await service.sendTest({ recipient: 'admin@example.com', actorId: 1 })

    await assert.rejects(
      () =>
        service.send({
          to: 'user@example.com',
          content: { subject: 'Test', html: '<p>Test</p>', text: 'Test' },
        }),
      (error) =>
        error instanceof MailConfigError &&
        error.code === 'provider_error' &&
        !error.message.includes('re_secret'),
    )
  })

  it('defaults enabled to true and carries it in the summary', async () => {
    const repository = createFakeRepository()
    const service = new MailConfigService(repository.port, { send: async () => {} }, secretCipher)

    const created = await service.update({
      apiKey: 're_runtime',
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 1,
    })

    assert.equal(created.enabled, true)
    assert.equal(repository.value?.enabled, true)
    assert.equal((await service.getSummary()).enabled, true)
  })

  it('persists an explicit enabled flag through saves', async () => {
    const repository = createFakeRepository()
    const service = new MailConfigService(repository.port, { send: async () => {} }, secretCipher)

    const disabled = await service.update({
      apiKey: 're_runtime',
      enabled: false,
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 1,
    })
    assert.equal(disabled.enabled, false)
    assert.equal(repository.value?.enabled, false)

    const updatedWhileDisabled = await service.update({
      fromName: 'IdeaBoard Mail',
      fromEmail: 'auth@example.com',
      actorId: 2,
    })
    assert.equal(updatedWhileDisabled.enabled, false)
    assert.equal(repository.value?.enabled, false)
    assert.equal(repository.value?.fromName, 'IdeaBoard Mail')

    const reEnabled = await service.update({
      enabled: true,
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 2,
    })
    assert.equal(reEnabled.enabled, true)
    assert.equal(repository.value?.enabled, true)
  })

  it('refuses test and transactional sends while disabled without touching the transport', async () => {
    const repository = createFakeRepository()
    let sends = 0
    const service = new MailConfigService(
      repository.port,
      {
        send: async () => {
          sends += 1
        },
      },
      secretCipher,
    )
    await service.update({
      apiKey: 're_runtime',
      enabled: false,
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 1,
    })

    await assert.rejects(
      () => service.sendTest({ recipient: 'admin@example.com', actorId: 2 }),
      (error) =>
        error instanceof MailConfigError &&
        error.code === 'mail_disabled' &&
        error.message.includes('邮件服务未启用'),
    )
    await assert.rejects(
      () =>
        service.send({
          to: 'user@example.com',
          content: { subject: 'Test', html: '<p>Test</p>', text: 'Test' },
        }),
      (error) =>
        error instanceof MailConfigError &&
        error.code === 'mail_disabled' &&
        error.message.includes('邮件服务未启用'),
    )
    assert.equal(sends, 0)
  })

  it('allows sends again after re-enabling and passing a test', async () => {
    const repository = createFakeRepository()
    let sends = 0
    const service = new MailConfigService(
      repository.port,
      {
        send: async () => {
          sends += 1
        },
      },
      secretCipher,
    )
    await service.update({
      apiKey: 're_runtime',
      enabled: false,
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 1,
    })
    await service.update({
      enabled: true,
      fromName: 'IdeaBoard',
      fromEmail: 'auth@example.com',
      actorId: 2,
    })

    await service.sendTest({ recipient: 'admin@example.com', actorId: 2 })
    await service.send({
      to: 'user@example.com',
      content: { subject: 'Test', html: '<p>Test</p>', text: 'Test' },
    })

    assert.equal(sends, 2)
    assert.equal(repository.value?.readiness, 'ready')
  })
})

function createFakeRepository() {
  const state: {
    value: StoredMailConfig | null
    nextVersion: number
    audits: Array<{
      action: string
      actorId: number
      metadata?: Record<string, unknown>
    }>
  } = { value: null, nextVersion: 1, audits: [] }
  const controls: {
    beforePut?: () => Promise<void>
    markTestError?: Error
    markTestCalls: number
  } = { markTestCalls: 0 }
  const port: MailConfigRepository = {
    async get() {
      return state.value ? { ...state.value } : null
    },
    async put(value, expectedConfigVersion, actorId, action) {
      await controls.beforePut?.()
      if ((state.value?.configVersion ?? 0) !== expectedConfigVersion) {
        return null
      }
      state.value = {
        ...value,
        configVersion: state.nextVersion++,
        updatedAt: new Date('2026-08-05T10:00:00Z'),
        lastTestedAt: null,
        lastTestErrorCode: null,
      }
      state.audits.push({ action, actorId })
      return { ...state.value }
    },
    async markTest(
      readiness: Extract<MailReadiness, 'ready' | 'unavailable'>,
      errorCode,
      actorId,
      configVersion,
    ) {
      controls.markTestCalls += 1
      if (controls.markTestError) throw controls.markTestError
      assert.ok(state.value)
      if (state.value.configVersion !== configVersion) {
        state.audits.push({
          action: 'test_mail_failure',
          actorId,
          metadata: { errorCode: 'config_changed' },
        })
        return false
      }
      state.value = {
        ...state.value,
        readiness,
        lastTestedAt: new Date('2026-08-05T10:01:00Z'),
        lastTestErrorCode: errorCode,
      }
      state.audits.push({
        action: readiness === 'ready' ? 'test_mail_success' : 'test_mail_failure',
        actorId,
        metadata: { errorCode },
      })
      return true
    },
    async clear(expectedConfigVersion, actorId) {
      if ((state.value?.configVersion ?? 0) !== expectedConfigVersion) {
        return false
      }
      state.value = null
      state.audits.push({ action: 'delete_mail_config', actorId })
      return true
    },
  }
  return {
    port,
    get beforePut() {
      return controls.beforePut
    },
    set beforePut(value: (() => Promise<void>) | undefined) {
      controls.beforePut = value
    },
    get markTestError() {
      return controls.markTestError
    },
    set markTestError(value: Error | undefined) {
      controls.markTestError = value
    },
    get markTestCalls() {
      return controls.markTestCalls
    },
    get value() {
      return state.value
    },
    get audits() {
      return state.audits
    },
  }
}
