export interface OutboundPolicySetting {
  allow_hosts: string[]
}

export interface OutboundPolicyForm {
  allow_hosts: string
}

export interface UpdateOutboundPolicyRequest {
  allow_hosts: string[]
}

export const DEFAULT_OUTBOUND_POLICY_FORM: OutboundPolicyForm = {
  allow_hosts: '',
}

export function normalizeOutboundPolicyForm(
  setting: Partial<OutboundPolicySetting> | null | undefined,
): OutboundPolicyForm {
  return {
    allow_hosts: normalizeAllowHosts(setting?.allow_hosts).join('\n'),
  }
}

export function outboundPolicyFormToRequest(form: OutboundPolicyForm): UpdateOutboundPolicyRequest {
  return {
    allow_hosts: parseAllowHosts(form.allow_hosts),
  }
}

export function validateOutboundPolicyForm(form: OutboundPolicyForm): string {
  const hosts = form.allow_hosts
    .split(/\r?\n/)
    .map((host) => host.trim())
    .filter(Boolean)
  if (hosts.length > 128) return '私有主机最多支持 128 条'
  if (hosts.some((host) => !normalizeAllowHost(host))) {
    return '私有主机仅支持 hostname、IP 或 host:port，每行一个'
  }

  return ''
}

function parseAllowHosts(value: string): string[] {
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

function normalizeBracketedIPv6(host: string): string | null {
  const bracketedIPv6WithPort = host.match(/^\[([^\]]+)]:(\d+)$/)
  if (!bracketedIPv6WithPort) return null
  const [, address = '', port = ''] = bracketedIPv6WithPort
  if (!isValidIPv6(address) || !isValidPort(port)) return null
  return `[${address}]:${Number(port)}`
}

function normalizeHostWithPort(host: string): string | null {
  const colonCount = (host.match(/:/g) ?? []).length
  if (colonCount > 1) return isValidIPv6(host) ? host : null

  const [hostname = '', port = ''] = host.split(':')
  const normalizedHostname = normalizeHostnameOrIPv4(hostname.replace(/\.$/, ''))
  if (!normalizedHostname || !isValidPort(port)) return null
  return `${normalizedHostname}:${Number(port)}`
}

function normalizeAllowHost(value: string): string | null {
  const host = value.trim().toLowerCase()
  if (!host || host.length > 255 || host.includes('://') || /[\s/?#@\\]/.test(host)) {
    return null
  }

  if (host.startsWith('[')) return normalizeBracketedIPv6(host)
  if (host.includes(':')) return normalizeHostWithPort(host)

  return normalizeHostnameOrIPv4(host.replace(/\.$/, ''))
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
