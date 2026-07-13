import { validateAdminActionReason } from '../shared/validation/adminActionReasonContract'

export interface OperationalTaskRuntimeSetting {
  worker_concurrency: number
  task_timeout_seconds: number
  no_capacity_retry_seconds: number
  max_retry: number
}

export interface OperationalOutboundPolicySetting {
  allow_hosts: string[]
}

export interface OperationalEventRetentionSetting {
  nats_retention_days: number
}

export interface OperationalSettingsValue {
  task_runtime: OperationalTaskRuntimeSetting
  outbound_policy: OperationalOutboundPolicySetting
  event_retention: OperationalEventRetentionSetting
}

export interface OperationalSettingState<TSetting> {
  configured: TSetting
  effective: TSetting
  restart_required: boolean
}

export interface OperationalSettingsState {
  task_runtime: OperationalSettingState<OperationalTaskRuntimeSetting>
  outbound_policy: OperationalSettingState<OperationalOutboundPolicySetting>
  event_retention: OperationalSettingState<OperationalEventRetentionSetting>
}

export interface OperationalSettingsForm {
  task_runtime: {
    worker_concurrency: number | null
    task_timeout_seconds: number | null
    no_capacity_retry_seconds: number | null
    max_retry: number | null
  }
  outbound_allow_hosts: string
  nats_retention_days: number | null
  reason: string
}

export interface UpdateOperationalSettingsRequest extends OperationalSettingsValue {
  reason: string
}

export const DEFAULT_OPERATIONAL_SETTINGS_VALUE: OperationalSettingsValue = {
  task_runtime: {
    worker_concurrency: 5,
    task_timeout_seconds: 180,
    no_capacity_retry_seconds: 5,
    max_retry: 25,
  },
  outbound_policy: { allow_hosts: [] },
  event_retention: { nats_retention_days: 7 },
}

export const DEFAULT_OPERATIONAL_SETTINGS_FORM: OperationalSettingsForm = {
  task_runtime: { ...DEFAULT_OPERATIONAL_SETTINGS_VALUE.task_runtime },
  outbound_allow_hosts: '',
  nats_retention_days: DEFAULT_OPERATIONAL_SETTINGS_VALUE.event_retention.nats_retention_days,
  reason: '',
}

export function normalizeOperationalSettingsForm(
  setting: Partial<OperationalSettingsValue> | null | undefined,
): OperationalSettingsForm {
  return {
    task_runtime: {
      worker_concurrency: normalizeInteger(
        setting?.task_runtime?.worker_concurrency,
        DEFAULT_OPERATIONAL_SETTINGS_VALUE.task_runtime.worker_concurrency,
      ),
      task_timeout_seconds: normalizeInteger(
        setting?.task_runtime?.task_timeout_seconds,
        DEFAULT_OPERATIONAL_SETTINGS_VALUE.task_runtime.task_timeout_seconds,
      ),
      no_capacity_retry_seconds: normalizeInteger(
        setting?.task_runtime?.no_capacity_retry_seconds,
        DEFAULT_OPERATIONAL_SETTINGS_VALUE.task_runtime.no_capacity_retry_seconds,
      ),
      max_retry: normalizeInteger(
        setting?.task_runtime?.max_retry,
        DEFAULT_OPERATIONAL_SETTINGS_VALUE.task_runtime.max_retry,
      ),
    },
    outbound_allow_hosts: normalizeAllowHosts(setting?.outbound_policy?.allow_hosts).join('\n'),
    nats_retention_days: normalizeInteger(
      setting?.event_retention?.nats_retention_days,
      DEFAULT_OPERATIONAL_SETTINGS_VALUE.event_retention.nats_retention_days,
    ),
    reason: '',
  }
}

export function operationalSettingsFormToRequest(
  form: OperationalSettingsForm,
): UpdateOperationalSettingsRequest {
  return {
    task_runtime: {
      worker_concurrency: form.task_runtime.worker_concurrency ?? 0,
      task_timeout_seconds: form.task_runtime.task_timeout_seconds ?? 0,
      no_capacity_retry_seconds: form.task_runtime.no_capacity_retry_seconds ?? 0,
      max_retry: form.task_runtime.max_retry ?? 0,
    },
    outbound_policy: { allow_hosts: parseAllowHosts(form.outbound_allow_hosts) },
    event_retention: { nats_retention_days: form.nats_retention_days ?? 0 },
    reason: form.reason.trim(),
  }
}

export function validateOperationalSettingsForm(form: OperationalSettingsForm): string {
  const taskRuntime = form.task_runtime
  if (!isIntegerWithin(taskRuntime.worker_concurrency, 1, 64)) {
    return 'Worker 并发必须在 1 到 64 之间'
  }
  if (!isIntegerWithin(taskRuntime.task_timeout_seconds, 30, 3600)) {
    return 'Task 超时必须在 30 到 3600 秒之间'
  }
  if (!isIntegerWithin(taskRuntime.no_capacity_retry_seconds, 1, 3600)) {
    return '无容量重试间隔必须在 1 到 3600 秒之间'
  }
  if (!isIntegerWithin(taskRuntime.max_retry, 1, 100)) {
    return '最大重试次数必须在 1 到 100 之间'
  }
  if (!isIntegerWithin(form.nats_retention_days, 1, 3650)) {
    return 'NATS 留存天数必须在 1 到 3650 天之间'
  }

  const hosts = form.outbound_allow_hosts
    .split(/\r?\n/)
    .map((host) => host.trim())
    .filter(Boolean)
  if (hosts.length > 128) return '出站白名单最多支持 128 条'
  if (hosts.some((host) => !normalizeAllowHost(host))) {
    return '出站白名单仅支持 hostname、IP 或 host:port，每行一个'
  }

  return validateAdminActionReason(form.reason).error
}

export function parseAllowHosts(value: string): string[] {
  return normalizeAllowHosts(value.split(/\r?\n/))
}

function normalizeAllowHosts(hosts: unknown): string[] {
  if (!Array.isArray(hosts)) return []

  const normalized: string[] = []
  const seen = new Set<string>()
  for (const candidate of hosts) {
    if (typeof candidate !== 'string') continue
    const host = normalizeAllowHost(candidate)
    if (!host || seen.has(host)) continue
    seen.add(host)
    normalized.push(host)
  }
  return normalized
}

function normalizeAllowHost(value: string): string | null {
  let host = value.trim().toLowerCase()
  if (!host || host.length > 255 || host.includes('://') || /[\s/?#@\\]/.test(host)) {
    return null
  }

  const bracketedIPv6WithPort = host.match(/^\[([^\]]+)]:(\d+)$/)
  if (bracketedIPv6WithPort) {
    const [, address = '', port = ''] = bracketedIPv6WithPort
    if (!isValidIPv6(address) || !isValidPort(port)) return null
    return `[${address}]:${Number(port)}`
  }

  if (host.includes(':')) {
    const colonCount = (host.match(/:/g) ?? []).length
    if (colonCount > 1) return isValidIPv6(host) ? host : null

    const [hostname = '', port = ''] = host.split(':')
    const normalizedHostname = normalizeHostnameOrIPv4(hostname.replace(/\.$/, ''))
    if (!normalizedHostname || !isValidPort(port)) return null
    return `${normalizedHostname}:${Number(port)}`
  }

  host = host.replace(/\.$/, '')
  return normalizeHostnameOrIPv4(host)
}

function normalizeHostnameOrIPv4(value: string): string | null {
  if (isValidIPv4(value)) return value
  if (value.length > 253) return null
  const labels = value.split('.')
  if (
    labels.some(
      (label) =>
        !label ||
        label.length > 63 ||
        label.startsWith('-') ||
        label.endsWith('-') ||
        !/^[a-z0-9-]+$/.test(label),
    )
  ) {
    return null
  }
  return value
}

function isValidIPv4(value: string): boolean {
  const parts = value.split('.')
  return (
    parts.length === 4 &&
    parts.every((part) => /^\d{1,3}$/.test(part) && Number(part) >= 0 && Number(part) <= 255)
  )
}

function isValidIPv6(value: string): boolean {
  try {
    const parsed = new URL(`http://[${value}]/`)
    return parsed.hostname.startsWith('[') && parsed.hostname.endsWith(']')
  } catch {
    return false
  }
}

function isValidPort(value: string): boolean {
  return /^\d+$/.test(value) && Number(value) >= 1 && Number(value) <= 65535
}

function isIntegerWithin(value: number | null, min: number, max: number): boolean {
  return value != null && Number.isInteger(value) && value >= min && value <= max
}

function normalizeInteger(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isInteger(value) ? value : fallback
}
