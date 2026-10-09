import {
  buildControlMutationHeaders,
  requireOperatorReason,
  type ControlMutationOptions,
} from '../../shared/lib/controlMutation'
import type { ControlMailConfigRequest } from '../../../../shared/api/generated-query/control/model/controlMailConfigRequest'
import { loadControlSetting, storeControlSetting } from '../../shared/lib/controlResource'
import {
  deleteControlMailSetting,
  getControlMailSetting,
  testControlMailSetting,
  updateControlMailSetting,
} from '../../../../shared/api/generated-query/control/control-platform/control-platform'
import { requireContractData } from '../../../../shared/api/contractKernel'
import type { MailConfigSummary, MailConfigUpdate } from './types'
import { withControlCopy } from '../../shared/lib/controlCopy'

const MAIL_CONTROL_MESSAGES = {
  forbidden: '权限不足，无法操作邮件配置',
  403: '权限不足，无法操作邮件配置',
  control_not_found: '邮件配置不存在',
  not_found: '邮件配置不存在',
  404: '邮件配置不存在',
  service_unavailable: '邮件服务暂不可用，请稍后重试',
  503: '邮件服务暂不可用，请稍后重试',
} as const

// Mail configuration uses Control Plane envelopes and audited mutations.
// Idempotency-Key is header-only; operator reason also accompanies body writes.
export type MailConfigMutationOptions = ControlMutationOptions

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
  return loadControlSetting(getControlMailSetting, {
    fallback: '加载邮件配置失败',
    domainMessages: MAIL_CONTROL_MESSAGES,
    map: (summary: MailConfigSummary) => summary,
  })
}

async function updateMailConfig(
  input: MailConfigUpdate,
  options: MailConfigMutationOptions,
): Promise<MailConfigSummary> {
  return storeControlSetting(input, options, {
    fallback: '保存邮件配置失败',
    domainMessages: MAIL_CONTROL_MESSAGES,
    map: (summary: MailConfigSummary) => summary,
    toRequest: (update: MailConfigUpdate, reason: string) => ({ ...update, reason }),
    persist: (body, headers) =>
      updateControlMailSetting(body as ControlMailConfigRequest, { headers }),
  })
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
