import { buildControlMutationHeaders, requireOperatorReason } from '../shared/lib/controlMutation'
import {
  deleteControlMailSetting,
  getControlMailSetting,
  testControlMailSetting,
  updateControlMailSetting,
} from '../../../shared/api/generated-query/control/control-platform/control-platform'
import { requireContractData } from '../../../shared/api/contractKernel'
import type { MailConfigSummary, MailConfigUpdate } from './mailConfigTypes'
import { withControlCopy } from '../shared/lib/controlCopy'

const MAIL_CONTROL_MESSAGES = {
  forbidden: '权限不足，无法操作邮件配置',
  403: '权限不足，无法操作邮件配置',
  control_not_found: '邮件配置不存在',
  not_found: '邮件配置不存在',
  404: '邮件配置不存在',
  service_unavailable: '邮件服务暂不可用，请稍后重试',
  503: '邮件服务暂不可用，请稍后重试',
} as const

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
    throw withControlCopy(error, '加载邮件配置失败', MAIL_CONTROL_MESSAGES)
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
      { headers: buildControlMutationHeaders(options) },
    )
    return requireContractData<MailConfigSummary>(body, '保存邮件配置失败')
  } catch (error) {
    throw withControlCopy(error, '保存邮件配置失败', MAIL_CONTROL_MESSAGES)
  }
}

async function clearMailConfig(options: MailConfigMutationOptions): Promise<void> {
  try {
    const body = await deleteControlMailSetting({
      headers: buildControlMutationHeaders(options),
    })
    requireContractData<{ deleted?: boolean | undefined }>(body, '删除邮件配置失败')
  } catch (error) {
    throw withControlCopy(error, '删除邮件配置失败', MAIL_CONTROL_MESSAGES)
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
      { headers: buildControlMutationHeaders(options) },
    )
    requireContractData<{ sent?: boolean | undefined }>(body, '测试邮件发送失败')
  } catch (error) {
    throw withControlCopy(error, '测试邮件发送失败', MAIL_CONTROL_MESSAGES)
  }
}
