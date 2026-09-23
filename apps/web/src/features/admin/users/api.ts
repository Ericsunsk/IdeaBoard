// User administration commands transport (issue 808): feature-owned
// boundary over the Control generated client. Mutations hit
// /api/control/v1 with a header-only Idempotency-Key and an operator
// reason (body + X-Operator-Reason header); responses use the contract
// envelope. Control error codes branch here so callers never match on
// copy: control_reason_required, idempotency_conflict,
// billing_wallet_restricted, control_not_found.
import {
  banControlUser,
  deleteControlUser,
  impersonateControlUser,
  revokeControlUserSessions,
  unbanControlUser,
  updateControlUserRole,
} from '../../../shared/api/generated-query/control/control-users/control-users'
import {
  buildContractRequestHeaders,
  newRequestId,
  requireContractData,
} from '../../../shared/api/contractKernel'
import { controlUserIdempotencyKey } from '../../../shared/api/idempotencyKey'

type ControlMutationOptions = {
  operatorReason: string
  idempotencyKey?: string | undefined
}

function requireOperatorReason(reason: string): string {
  const trimmed = reason.trim()
  if (!trimmed) throw controlError('control_reason_required', '需要填写操作原因')
  return trimmed
}

function controlError(code: string, message: string): Error & { code?: string | undefined } {
  const error = new Error(message) as Error & { code?: string | undefined }
  error.code = code
  return error
}

function mutationHeaders(
  operation: Parameters<typeof controlUserIdempotencyKey>[0],
  options: ControlMutationOptions,
) {
  return buildContractRequestHeaders({
    requestId: newRequestId(),
    idempotencyKey: options.idempotencyKey ?? controlUserIdempotencyKey(operation),
    operatorReason: requireOperatorReason(options.operatorReason),
  })
}

async function resolveControlOk(request: Promise<unknown>, fallback: string): Promise<void> {
  try {
    requireContractData<unknown>(await request, fallback)
  } catch (error) {
    throw withControlCopy(error, fallback)
  }
}

// withControlCopy maps control error codes to operator-facing copy.
// Branch on code, never on message text.
function withControlCopy(error: unknown, fallback: string): Error {
  const code = readErrorCode(error)
  if (code === 'idempotency_conflict') {
    return controlError(code, '操作已提交，请勿重复操作')
  }
  if (code === 'control_reason_required') {
    return controlError(code, '需要填写操作原因')
  }
  if (code === 'control_not_found') {
    return controlError(code, '用户不存在')
  }
  if (code === 'billing_wallet_restricted') {
    return controlError(code, '账户受限，暂时无法操作')
  }
  if (error instanceof Error) return error
  return controlError('control_failed', fallback)
}

function readErrorCode(error: unknown): string | undefined {
  if (error && typeof error === 'object' && 'code' in error) {
    const code = (error as { code?: unknown }).code
    return typeof code === 'string' ? code : undefined
  }
  return undefined
}

export async function banAdminUser(input: {
  publicId: string
  reason?: string | undefined
  expires_in_days?: number | undefined
  operatorReason: string
  idempotencyKey?: string | undefined
}): Promise<void> {
  const operatorReason = requireOperatorReason(input.operatorReason)
  await resolveControlOk(
    banControlUser(
      input.publicId,
      {
        ...(input.reason?.trim() ? { reason: input.reason.trim() } : {}),
        ...(input.expires_in_days === undefined ? {} : { expires_in_days: input.expires_in_days }),
      },
      { headers: mutationHeaders('ban', { operatorReason, idempotencyKey: input.idempotencyKey }) },
    ),
    '封禁失败',
  )
}

export async function unbanAdminUser(
  publicId: string,
  options: ControlMutationOptions,
): Promise<void> {
  await resolveControlOk(
    unbanControlUser(publicId, {
      headers: mutationHeaders('unban', options),
    }),
    '解封失败',
  )
}

export async function revokeAdminUserSessions(
  publicId: string,
  options: ControlMutationOptions,
): Promise<void> {
  await resolveControlOk(
    revokeControlUserSessions(publicId, {
      headers: mutationHeaders('revoke-sessions', options),
    }),
    '吊销会话失败',
  )
}

export async function updateAdminUserRole(input: {
  publicId: string
  role: 'admin' | 'user'
  operatorReason: string
  idempotencyKey?: string | undefined
}): Promise<void> {
  await resolveControlOk(
    updateControlUserRole(
      input.publicId,
      { role: input.role },
      {
        headers: mutationHeaders('role', {
          operatorReason: input.operatorReason,
          idempotencyKey: input.idempotencyKey,
        }),
      },
    ),
    '角色更新失败',
  )
}

export async function deleteAdminUser(
  publicId: string,
  options: ControlMutationOptions,
): Promise<void> {
  await resolveControlOk(
    deleteControlUser(publicId, {
      headers: mutationHeaders('delete', options),
    }),
    '注销失败',
  )
}

export async function impersonateAdminUser(
  publicId: string,
  options: ControlMutationOptions,
): Promise<void> {
  await resolveControlOk(
    impersonateControlUser(publicId, {
      headers: mutationHeaders('impersonate', options),
    }),
    '伪装登录失败',
  )
}
