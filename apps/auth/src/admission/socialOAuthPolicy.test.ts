import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { APIError } from 'better-auth/api'
import {
  applySocialOAuthRequestPolicy,
  createRegistrationUserCreationGuard,
} from './socialOAuthPolicy.js'
import { createAuthAdmissionPolicy } from './authAdmissionPolicy.js'

function createPolicy(getRegistrationEnabled: () => Promise<boolean>, providerReady = true) {
  return createAuthAdmissionPolicy({
    trustedOrigins: [],
    onDependencyError: () => {},
    getSettings: async () => ({
      registrationEnabled: await getRegistrationEnabled(),
      windowSeconds: 60,
      maxRequests: 30,
      lockoutThreshold: 10,
      lockoutDurationSeconds: 900,
    }),
    isMailReady: async () => true,
    getSocialProviderReadiness: async () => providerReady,
  })
}

describe('Social OAuth policy', () => {
  for (const { provider, scopes, requestSignUp } of [
    {
      provider: 'google',
      scopes: ['https://www.googleapis.com/auth/drive'],
      requestSignUp: false,
    },
    { provider: 'discord', scopes: ['guilds'], requestSignUp: true },
  ]) {
    it(`removes caller scopes and derives sign-up intent from server policy (${provider})`, async () => {
      const context = {
        path: '/sign-in/social',
        body: {
          provider,
          scopes: [...scopes],
          requestSignUp,
        },
      }

      await applySocialOAuthRequestPolicy(
        context,
        createPolicy(async () => true),
      )

      assert.equal('scopes' in context.body, false)
      assert.equal(context.body.requestSignUp, true)
    })
  }

  it('removes caller scopes from account linking without changing other providers', async () => {
    const google = {
      path: '/link-social',
      body: { provider: 'google', scopes: ['drive'] },
    }
    const github = {
      path: '/sign-in/social',
      body: { provider: 'github', scopes: ['repo'] },
    }

    const policy = createPolicy(async () => true)
    await applySocialOAuthRequestPolicy(google, policy)
    await applySocialOAuthRequestPolicy(github, policy)

    assert.equal('scopes' in google.body, false)
    assert.deepEqual(github.body.scopes, ['repo'])
  })

  it('checks current registration policy at verified OAuth user creation', async () => {
    const disabledGuard = createRegistrationUserCreationGuard(createPolicy(async () => false))
    const enabledGuard = createRegistrationUserCreationGuard(createPolicy(async () => true))

    await assert.rejects(
      () => disabledGuard({ emailVerified: true }),
      (error) => error instanceof APIError && error.statusCode === 403,
    )
    await assert.rejects(
      () => disabledGuard({ emailVerified: true }, { path: '/sign-in/email-otp' }),
      (error) => error instanceof APIError && error.statusCode === 403,
    )
    assert.equal(await enabledGuard({ emailVerified: true }), undefined)
    assert.equal(await disabledGuard({ emailVerified: false }), undefined)
    assert.equal(
      await enabledGuard({ emailVerified: true }, { path: '/sign-in/email-otp' }),
      undefined,
    )
  })

  it('fails closed when registration policy cannot be read', async () => {
    const policy = createPolicy(async () => {
      throw new Error('database unavailable')
    })
    const guard = createRegistrationUserCreationGuard(policy)
    const context = {
      path: '/sign-in/social',
      body: { provider: 'google', requestSignUp: true },
    }

    await applySocialOAuthRequestPolicy(context, policy)

    assert.equal(context.body.requestSignUp, false)
    await assert.rejects(
      () => guard({ emailVerified: true }),
      (error) => error instanceof APIError && error.statusCode === 503,
    )
  })
})
