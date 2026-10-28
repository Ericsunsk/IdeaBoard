import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  createTrustedIdentityProvisioner,
  sessionIdentityFromAuthUser,
  TrustedIdentityProvisioningError,
} from './trustedIdentity.js'

test('session identity adapter normalizes a verified canonical email', () => {
  const identity = sessionIdentityFromAuthUser({
    id: ' auth-1 ',
    email: ' Alice@Example.com ',
    emailVerified: true,
    name: '  Alice 设计师  ',
    image: 'https://lh3.googleusercontent.com/avatar',
  })

  assert.deepEqual(identity, {
    auth_user_id: 'auth-1',
    email: 'alice@example.com',
    display_name: 'Alice 设计师',
    avatar: 'https://lh3.googleusercontent.com/avatar',
    claim_identity: 'alice@example.com',
    claim_identity_kind: 'canonical_email',
    claim_identity_verified: true,
  })
})

test('session identity adapter preserves fail-closed state for an untrusted claim', () => {
  const identity = sessionIdentityFromAuthUser({
    id: 'auth-2',
    email: 'not-an-email',
    emailVerified: true,
    name: 'Alice',
  })

  assert.equal(identity.claim_identity_kind, 'untrusted')
  assert.equal(identity.claim_identity_verified, false)
  assert.equal(identity.avatar, 'https://api.dicebear.com/9.x/big-ears-neutral/svg?seed=auth-2')
})

test('session identity adapter assigns a deterministic avatar to email users', () => {
  const identity = sessionIdentityFromAuthUser({
    id: 'auth-3',
    email: 'alice@example.com',
    emailVerified: true,
    name: 'Alice',
  })
  assert.equal(identity.avatar, 'https://api.dicebear.com/9.x/big-ears-neutral/svg?seed=auth-3')
})

describe('TrustedIdentityProvisioner', () => {
  test('creates and finalizes one normalized canonical identity', async () => {
    const finalized: unknown[] = []
    const provisioner = createTrustedIdentityProvisioner(
      {
        signUpEmail: async ({ body }) => ({
          user: { id: 'auth-1', email: body.email, name: body.name },
        }),
      },
      {
        findUserByEmail: async () => null,
        finalizeTrustedRegistrationIdentity: async (authUserID, claimIdentity, role) => {
          finalized.push({ authUserID, claimIdentity, role })
          return true
        },
        removeAuthUserCascade: async () => true,
      },
    )

    assert.deepEqual(
      await provisioner.provision({
        email: ' Admin@Example.com ',
        displayName: ' 管理员 ',
        password: 'secret1234',
        role: 'ADMIN',
        allowExisting: false,
      }),
      {
        kind: 'created',
        user: {
          id: 'auth-1',
          email: 'admin@example.com',
          name: '管理员',
        },
        identity: {
          claimIdentity: 'admin@example.com',
          displayName: '管理员',
          role: 'admin',
        },
      },
    )
    assert.deepEqual(finalized, [
      {
        authUserID: 'auth-1',
        claimIdentity: 'admin@example.com',
        role: 'admin',
      },
    ])
  })

  test('returns typed existing and mismatch outcomes after duplicate signup', async () => {
    let role = 'admin'
    const provisioner = createTrustedIdentityProvisioner(
      {
        signUpEmail: async () => {
          throw new Error('user already exists')
        },
      },
      {
        findUserByEmail: async () => ({
          id: 'auth-existing',
          email: 'admin@example.com',
          emailVerified: true,
          name: 'Existing Admin',
          role,
        }),
        finalizeTrustedRegistrationIdentity: async () => true,
        removeAuthUserCascade: async () => true,
      },
    )
    const input = {
      email: 'admin@example.com',
      displayName: '管理员',
      password: 'secret1234',
      role: 'admin',
      allowExisting: true,
    }

    assert.equal((await provisioner.provision(input)).kind, 'existing')
    role = 'user'
    assert.deepEqual(await provisioner.provision(input), {
      kind: 'conflict',
      reason: 'identity_mismatch',
    })
  })

  test('returns a reconciliation signal when compensation fails', async () => {
    const cleanupError = new Error('delete unavailable')
    const provisioner = createTrustedIdentityProvisioner(
      {
        signUpEmail: async ({ body }) => ({
          user: { id: 'auth-partial', email: body.email, name: body.name },
        }),
      },
      {
        findUserByEmail: async () => null,
        finalizeTrustedRegistrationIdentity: async () => false,
        removeAuthUserCascade: async () => {
          throw cleanupError
        },
      },
    )

    await assert.rejects(
      provisioner.provision({
        email: 'admin@example.com',
        displayName: '管理员',
        password: 'secret1234',
        role: 'admin',
      }),
      (error: unknown) => {
        assert.ok(error instanceof TrustedIdentityProvisioningError)
        assert.equal(error.code, 'reconciliation_required')
        assert.equal(error.authUserID, 'auth-partial')
        assert.ok(error.cause instanceof AggregateError)
        assert.equal(error.cause.errors[1], cleanupError)
        return true
      },
    )
  })

  test('treats an already-absent cleanup target as converged', async () => {
    const provisioner = createTrustedIdentityProvisioner(
      {
        signUpEmail: async ({ body }) => ({
          user: { id: 'auth-partial', email: body.email, name: body.name },
        }),
      },
      {
        findUserByEmail: async () => null,
        finalizeTrustedRegistrationIdentity: async () => false,
        removeAuthUserCascade: async () => false,
      },
    )

    await assert.rejects(
      provisioner.provision({
        email: 'admin@example.com',
        displayName: '管理员',
        password: 'secret1234',
        role: 'admin',
      }),
      (error: unknown) => {
        assert.ok(error instanceof TrustedIdentityProvisioningError)
        assert.equal(error.code, 'dependency_failure')
        assert.equal(error.authUserID, 'auth-partial')
        return true
      },
    )
  })

  test('recovers and compensates an identity when signup returns no payload', async () => {
    const cleaned: string[] = []
    const provisioner = createTrustedIdentityProvisioner(
      { signUpEmail: async () => ({}) },
      {
        findUserByEmail: async () => ({
          id: 'auth-recovered',
          email: 'admin@example.com',
        }),
        finalizeTrustedRegistrationIdentity: async () => true,
        removeAuthUserCascade: async (authUserID) => {
          cleaned.push(authUserID)
          return true
        },
      },
    )

    await assert.rejects(
      provisioner.provision({
        email: 'admin@example.com',
        displayName: '管理员',
        password: 'secret1234',
        role: 'admin',
      }),
      (error: unknown) => {
        assert.ok(error instanceof TrustedIdentityProvisioningError)
        assert.equal(error.code, 'dependency_failure')
        assert.equal(error.authUserID, 'auth-recovered')
        return true
      },
    )
    assert.deepEqual(cleaned, ['auth-recovered'])
  })

  test('requires reconciliation when an empty signup payload cannot be recovered', async () => {
    const lookupError = new Error('lookup unavailable')
    const provisioner = createTrustedIdentityProvisioner(
      { signUpEmail: async () => ({}) },
      {
        findUserByEmail: async () => {
          throw lookupError
        },
        finalizeTrustedRegistrationIdentity: async () => true,
        removeAuthUserCascade: async () => true,
      },
    )

    await assert.rejects(
      provisioner.provision({
        email: 'admin@example.com',
        displayName: '管理员',
        password: 'secret1234',
        role: 'admin',
      }),
      (error: unknown) => {
        assert.ok(error instanceof TrustedIdentityProvisioningError)
        assert.equal(error.code, 'reconciliation_required')
        assert.equal(error.authUserID, undefined)
        assert.equal(error.cause, lookupError)
        return true
      },
    )
  })
})
