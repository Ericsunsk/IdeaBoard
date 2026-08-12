const SENSITIVE_VALUE_PATTERN =
  /\b(?:sk|rk|pk)_(?:live|test)_[a-z0-9_-]+\b|\bwhsec_[a-z0-9_-]+\b|\bbearer\s+[a-z0-9._~+/=-]+|\b(?:token|secret|password|cookie|authorization|api[-_\s]?key|signature|credential)\s*[:=]\s*\S+/i
const INTERNAL_ERROR_PATTERN =
  /api 返回 \d+|http 请求失败|读取 api 响应失败|解析响应失败|dial tcp|lookup |connection refused|context deadline exceeded|no such host|i\/o timeout|tls:|x509:|proxyconnect tcp|broken pipe|unexpected eof/i
const MAX_SAFE_ERROR_LENGTH = 160

function normalizeMessage(message: string | null | undefined): string {
  return typeof message === 'string' ? message.trim() : ''
}

export function sanitizeApiErrorMessage(
  message: string | null | undefined,
  fallback: string,
): string {
  const normalized = normalizeMessage(message)
  if (!normalized) return fallback
  if (normalized.length > MAX_SAFE_ERROR_LENGTH) return fallback
  if (SENSITIVE_VALUE_PATTERN.test(normalized) || INTERNAL_ERROR_PATTERN.test(normalized))
    return fallback
  return normalized
}

export function extractSafeApiErrorMessage(error: unknown, fallback: string): string {
  if (
    typeof error === 'object' &&
    error &&
    'msg' in error &&
    typeof (error as { msg?: unknown }).msg === 'string'
  ) {
    return sanitizeApiErrorMessage((error as { msg: string }).msg, fallback)
  }
  return fallback
}

export function sanitizeTaskErrorMessage(
  message: string | null | undefined,
  fallback = '上游图像服务调用失败',
): string {
  return sanitizeApiErrorMessage(message, fallback)
}

/** Unified display helper: prefers Error.message first, then falls back to API-safe extraction. */
export function formatDisplayErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error) {
    if (
      'hasSafeDisplayMessage' in error &&
      (error as Error & { hasSafeDisplayMessage?: unknown }).hasSafeDisplayMessage === false
    ) {
      return fallback
    }
    return sanitizeApiErrorMessage(error.message, fallback)
  }
  return extractSafeApiErrorMessage(error, fallback)
}
