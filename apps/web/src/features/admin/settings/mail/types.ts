export type MailReadiness = 'not_configured' | 'not_tested' | 'ready' | 'unavailable'

export type MailConfigSummary = {
  configured: boolean
  api_key_configured: boolean
  /** 关闭后停止一切外发，凭据保留。 */
  enabled: boolean
  from_name: string
  from_email: string
  reply_to: string
  config_version: number
  readiness: MailReadiness
  updated_at: string | null
  last_tested_at: string | null
  last_test_error_code: string | null
}

export type MailConfigUpdate = {
  api_key?: string
  from_name: string
  from_email: string
  reply_to?: string
  /** 关闭后停止一切外发，凭据保留；省略时保持原值。 */
  enabled?: boolean
}
