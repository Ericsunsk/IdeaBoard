import { api } from '../../../shared/api/client'
import { resolveApiData, resolveApiOk } from '../../../shared/api/apiResponse'
import type { MailConfigSummary, MailConfigUpdate } from './mailConfigTypes'

export interface AdminMailConfigResource {
  get: () => Promise<MailConfigSummary>
  update: (input: MailConfigUpdate) => Promise<MailConfigSummary>
  clear: () => Promise<void>
  test: (recipient: string) => Promise<void>
}

export function createAdminMailConfigResource(): AdminMailConfigResource {
  return {
    get: getMailConfig,
    update: updateMailConfig,
    clear: clearMailConfig,
    test: testMailConfig,
  }
}

function getMailConfig() {
  return resolveApiData<MailConfigSummary>(api.request('/admin/settings/mail'), '加载邮件配置失败')
}

function updateMailConfig(input: MailConfigUpdate) {
  return resolveApiData<MailConfigSummary>(
    api.request('/admin/settings/mail', { method: 'PUT', body: input }),
    '保存邮件配置失败',
  )
}

function clearMailConfig() {
  return resolveApiOk(
    api.request('/admin/settings/mail', { method: 'DELETE', body: {} }),
    '删除邮件配置失败',
  )
}

function testMailConfig(recipient: string) {
  return resolveApiOk(
    api.request('/admin/settings/mail/test', { method: 'POST', body: { recipient } }),
    '测试邮件发送失败',
  )
}
