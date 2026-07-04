import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  runUpdateAuthUserRoleCommand,
  type UpdateAuthUserRoleRepository,
} from './authInternalCommands.js'

describe('runUpdateAuthUserRoleCommand', () => {
  it('normalizes input, updates the repository, and returns the route response data', async () => {
    const updates: Array<{ authUserID: string; role: 'admin' | 'user' }> = []
    const repository: UpdateAuthUserRoleRepository = {
      updateAuthUserRole: async (authUserID, role) => {
        updates.push({ authUserID, role })
        return true
      },
    }

    const result = await runUpdateAuthUserRoleCommand(
      { authUserID: ' auth-user-1 ', role: 'ADMIN' },
      repository,
    )

    assert.deepEqual(updates, [{ authUserID: 'auth-user-1', role: 'admin' }])
    assert.deepEqual(result, {
      ok: true,
      data: {
        auth_user_id: 'auth-user-1',
        role: 'admin',
      },
    })
  })

  it('keeps the existing default role behavior when role is omitted', async () => {
    const updates: Array<{ authUserID: string; role: 'admin' | 'user' }> = []
    const repository: UpdateAuthUserRoleRepository = {
      updateAuthUserRole: async (authUserID, role) => {
        updates.push({ authUserID, role })
        return true
      },
    }

    const result = await runUpdateAuthUserRoleCommand({ authUserID: 'auth-user-1' }, repository)

    assert.deepEqual(updates, [{ authUserID: 'auth-user-1', role: 'user' }])
    assert.deepEqual(result, {
      ok: true,
      data: {
        auth_user_id: 'auth-user-1',
        role: 'user',
      },
    })
  })

  it('returns validation errors before calling the repository', async () => {
    let repositoryCalled = false
    const repository: UpdateAuthUserRoleRepository = {
      updateAuthUserRole: async () => {
        repositoryCalled = true
        return true
      },
    }

    const result = await runUpdateAuthUserRoleCommand(
      { authUserID: ' ', role: 'admin' },
      repository,
    )

    assert.equal(repositoryCalled, false)
    assert.deepEqual(result, {
      ok: false,
      status: 400,
      message: 'auth user id is required',
    })
  })

  it('classifies invalid roles as bad requests before calling the repository', async () => {
    let repositoryCalled = false
    const repository: UpdateAuthUserRoleRepository = {
      updateAuthUserRole: async () => {
        repositoryCalled = true
        return true
      },
    }

    const result = await runUpdateAuthUserRoleCommand(
      { authUserID: 'auth-user-1', role: 'owner' },
      repository,
    )

    assert.equal(repositoryCalled, false)
    assert.deepEqual(result, {
      ok: false,
      status: 400,
      message: 'role must be admin or user',
    })
  })

  it('classifies a missing auth user as not found', async () => {
    const repository: UpdateAuthUserRoleRepository = {
      updateAuthUserRole: async () => false,
    }

    const result = await runUpdateAuthUserRoleCommand(
      { authUserID: 'auth-missing', role: 'user' },
      repository,
    )

    assert.deepEqual(result, {
      ok: false,
      status: 404,
      message: 'auth user not found',
    })
  })

  it('classifies repository failures as internal errors with the existing fallback message', async () => {
    const repository: UpdateAuthUserRoleRepository = {
      updateAuthUserRole: async () => {
        throw new Error('database unavailable')
      },
    }

    const result = await runUpdateAuthUserRoleCommand(
      { authUserID: 'auth-user-1', role: 'admin' },
      repository,
    )

    assert.deepEqual(result, {
      ok: false,
      status: 500,
      message: 'database unavailable',
    })
  })
})
