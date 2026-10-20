import { updateUser } from '../../../../api/generated-query/public/user/user'
import type { UpdateUserRequest } from '../../../../api/generated-query/public/model'
import {
  buildContractRequestHeaders,
  newRequestId,
  requireContractData,
} from '../../../../api/contractKernel'
import { toSessionUser } from '../../../../api/userSession'
import type { User } from '../../../../api/sessionUserType'
import { i18n } from '../../../../core/i18n'
import { withSignal } from '../../../../core/utils/request'

// Match the OpenAPI and PostgreSQL limit in Unicode code points.
const DISPLAY_NAME_MAX_LENGTH = 100

export function normalizeDisplayName(value: string): string {
  return value.trim()
}

export function validateDisplayName(value: string): string | null {
  const next = normalizeDisplayName(value)
  if (!next) return i18n.global.t('account.profile.displayNameEmpty')
  if (Array.from(next).length > DISPLAY_NAME_MAX_LENGTH) {
    return i18n.global.t('account.profile.displayNameTooLong', {
      max: DISPLAY_NAME_MAX_LENGTH,
    })
  }
  return null
}

export type DisplayNameRequestOptions = {
  signal?: AbortSignal | null | undefined
}

export async function renameDisplayName(
  displayName: string,
  options?: DisplayNameRequestOptions,
): Promise<User> {
  const violation = validateDisplayName(displayName)
  if (violation) throw new Error(violation)
  const body: UpdateUserRequest = { data: { display_name: normalizeDisplayName(displayName) } }
  const envelope = await updateUser(body, {
    ...withSignal(options?.signal),
    headers: buildContractRequestHeaders({ requestId: newRequestId() }),
  })
  const data = requireContractData<unknown>(
    envelope,
    i18n.global.t('account.toast.displayNameUpdateFailed'),
  )
  const user = toSessionUser(data)
  if (!user) throw new Error(i18n.global.t('account.toast.displayNameUpdateFailed'))
  return user
}
