import {
  deleteControlMailSetting,
  getControlMailSetting,
  testControlMailSetting,
  updateControlMailSetting,
} from '../../../shared/api/generated-query/control/control-platform/control-platform'
import {
  buildContractRequestHeaders,
  newRequestId,
  requireContractData,
} from '../../../shared/api/contractKernel'
import type { MailConfigSummary, MailConfigUpdate } from './mailConfigTypes'

// Admin mail config boundary (#809 leftovers): the full mail flow (read,
// save, clear, test) goes through the Control Plane mail-setting
// operations with the contract envelope (`{ data }`). Mutations carry a
// header-only Idempotency-Key and an operator reason (mirrored into the
// body `reason` field, which the control guard also accepts); every write
// emits a control audit record. The legacy `/api/admin/settings/mail*`
// routes are untouched server-side; this resource no longer calls them.
export type MailConfigMutationOptions = {
  operatorReason: string
  idempotencyKey?: string | undefined
}

export interface AdminMailConfigResource {
  get: () => Promise<MailConfigSummary>
  update: (
    input: MailConfigUpdate,
    options: MailConfigMutationOptions,
  ) => Promise<MailConfigSummary>
  clear: (options: MailConfigMutationOptions) => Promise<void>
  test: (recipient: string, options: MailConfigMutationOptions) => Promise<void>
}

export function createAdminMailConfigResource(): AdminMailConfigResource {
  return {
    get: getMailConfig,
    update: updateMailConfig,
    clear: clearMailConfig,
    test: testMailConfig,
  }
}

async function getMailConfig(): Promise<MailConfigSummary> {
  try {
    const body = await getControlMailSetting()
    return requireContractData<MailConfigSummary>(body, '加载邮件配置失败')
  } catch (error) {
    throw withControlCopy(error, '加载邮件配置失败')
  }
}

async function updateMailConfig(
  input: MailConfigUpdate,
  options: MailConfigMutationOptions,
): Promise<MailConfigSummary> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  try {
    const body = await updateControlMailSetting(
      { ...input, reason: operatorReason },
      { headers: mutationHeaders(options, operatorReason) },
    )
    return requireContractData<MailConfigSummary>(body, '保存邮件配置失败')
  } catch (error) {
    throw withControlCopy(error, '保存邮件配置失败')
  }
}

async function clearMailConfig(options: MailConfigMutationOptions): Promise<void> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  try {
    const body = await deleteControlMailSetting({
      headers: mutationHeaders(options, operatorReason),
    })
    requireContractData<{ deleted?: boolean | undefined }>(body, '删除邮件配置失败')
  } catch (error) {
    throw withControlCopy(error, '删除邮件配置失败')
  }
}

async function testMailConfig(
  recipient: string,
  options: MailConfigMutationOptions,
): Promise<void> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  try {
    const body = await testControlMailSetting(
      { recipient, reason: operatorReason },
      { headers: mutationHeaders(options, operatorReason) },
    )
    requireContractData<{ sent?: boolean | undefined }>(body, '测试邮件发送失败')
  } catch (error) {
    throw withControlCopy(error, '测试邮件发送失败')
  }
}

function mutationHeaders(options: MailConfigMutationOptions, operatorReason: string) {
  return buildContractRequestHeaders({
    requestId: newRequestId(),
    idempotencyKey: options.idempotencyKey ?? newRequestId(),
    operatorReason,
  })
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

// withControlCopy maps control error codes to operator-facing copy.
// Branch on code, never on message text. Reads never carry an operator
// reason, so `control_reason_required` only fires on writes; 403/404/503
// converge here for both reads and the write side.
function withControlCopy(error: unknown, fallback: string): Error {
  const code = readErrorCode(error)
  if (code === 'idempotency_conflict' || code === 409) {
    return controlError(code === 409 ? 'idempotency_conflict' : code, '操作已提交，请勿重复操作')
  }
  if (code === 'control_reason_required') {
    return controlError(code, '需要填写操作原因')
  }
  if (code === 'forbidden' || code === 403) {
    return controlError('forbidden', '权限不足，无法操作邮件配置')
  }
  if (code === 'control_not_found' || code === 'not_found' || code === 404) {
    return controlError('control_not_found', '邮件配置不存在')
  }
  if (code === 'service_unavailable' || code === 503) {
    return controlError('service_unavailable', '邮件服务暂不可用，请稍后重试')
  }
  if (error instanceof Error) return error
  return controlError('control_failed', fallback)
}

function readErrorCode(error: unknown): string | number | undefined {
  if (error && typeof error === 'object' && 'code' in error) {
    const code = (error as { code?: unknown }).code
    if (typeof code === 'string' || typeof code === 'number') return code
  }
  if (error && typeof error === 'object' && 'status' in error) {
    const status = (error as { status?: unknown }).status
    if (status === 403 || status === 404 || status === 503 || status === 409) return status
  }
  return undefined
}
