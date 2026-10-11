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
} from '../../../../api/generated-query/control/control-users/control-users'
import { requireContractData } from '../../../../api/contractKernel'
import { controlUserIdempotencyKey } from '../../../../api/idempotencyKey'
import {
  buildControlMutationHeaders,
  requireOperatorReason,
  type ControlMutationOptions,
} from '../../shared'
import { withControlCopy } from '../../shared'

function controlUserHeaders(
  operation: Parameters<typeof controlUserIdempotencyKey>[0],
  options: ControlMutationOptions,
) {
  return buildControlMutationHeaders(options, controlUserIdempotencyKey(operation))
}

async function resolveControlOk(request: Promise<unknown>, fallback: string): Promise<void> {
  try {
    requireContractData<unknown>(await request, fallback)
  } catch (error) {
    throw withControlCopy(error, fallback, {
      control_not_found: '用户不存在',
      billing_wallet_restricted: '账户受限，暂时无法操作',
    })
  }
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
      {
        headers: controlUserHeaders('ban', {
          operatorReason,
          idempotencyKey: input.idempotencyKey,
        }),
      },
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
      headers: controlUserHeaders('unban', options),
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
      headers: controlUserHeaders('revoke-sessions', options),
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
        headers: controlUserHeaders('role', {
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
      headers: controlUserHeaders('delete', options),
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
      headers: controlUserHeaders('impersonate', options),
    }),
    '伪装登录失败',
  )
}
