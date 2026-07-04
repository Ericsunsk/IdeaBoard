import { extractErrorMessage, normalizeAuthRole } from './authUtils.js'

type AuthRole = 'admin' | 'user'

export type AuthInternalCommandResult<TData> =
  | {
      ok: true
      data: TData
    }
  | {
      ok: false
      status: number
      message: string
    }

export type UpdateAuthUserRoleCommandInput = {
  authUserID?: unknown
  role?: unknown
}

export type UpdateAuthUserRoleRepository = {
  updateAuthUserRole(authUserID: string, role: AuthRole): Promise<boolean>
}

export type UpdateAuthUserRoleCommandData = {
  auth_user_id: string
  role: AuthRole
}

export async function runUpdateAuthUserRoleCommand(
  input: UpdateAuthUserRoleCommandInput,
  repository: UpdateAuthUserRoleRepository,
): Promise<AuthInternalCommandResult<UpdateAuthUserRoleCommandData>> {
  const authUserID = validateAuthUserID(input.authUserID)
  const role = normalizeAuthRole(input.role)

  if (authUserID.error) {
    return commandError(400, authUserID.error)
  }
  if (!role) {
    return commandError(400, 'role must be admin or user')
  }

  try {
    const updated = await repository.updateAuthUserRole(authUserID.value, role)
    if (!updated) {
      return commandError(404, 'auth user not found')
    }
    return {
      ok: true,
      data: {
        auth_user_id: authUserID.value,
        role,
      },
    }
  } catch (error) {
    return commandError(500, extractErrorMessage(error, 'failed to update auth user role'))
  }
}

function commandError(status: number, message: string): AuthInternalCommandResult<never> {
  return {
    ok: false,
    status,
    message,
  }
}

type AuthUserIDValidation = {
  value: string
  error: string
}

function validateAuthUserID(raw: unknown): AuthUserIDValidation {
  const value = String(raw ?? '').trim()
  if (!value) return { value: '', error: 'auth user id is required' }
  if (value.length > 191) return { value: '', error: 'auth user id is invalid' }
  return { value, error: '' }
}
