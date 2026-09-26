import { i18n } from '../i18n'

/**
 * Central catalog mapping stable backend error codes → User copy keys.
 *
 * Convergence rule: whenever an API payload carries a structured code
 * (`error_code`, typed error kinds), classification MUST go through this
 * module instead of matching message prose. Prose matchers are allowed only
 * as documented protocol shims where the backend does not send a code yet.
 */

// SubmitTaskReadinessData.error_code (see ApiResponseTaskSubmitReadinessDataErrorCode)
const USER_ERROR_CODE_COPY_KEY_MAP: Readonly<Record<string, string>> = {
  submission_invalid: 'errorCodes.submission_invalid',
  submission_failed: 'errorCodes.submission_failed',
  pricing_unavailable: 'errorCodes.pricing_unavailable',
  provider_unavailable: 'errorCodes.provider_unavailable',
  prompt_unavailable: 'errorCodes.prompt_unavailable',
  wallet_restricted: 'errorCodes.wallet_restricted',
  insufficient_credits: 'errorCodes.insufficient_credits',
  vision_search_not_configured: 'errorCodes.vision_search_not_configured',
  vision_search_unavailable: 'errorCodes.vision_search_unavailable',
  vision_search_input_invalid: 'errorCodes.vision_search_input_invalid',
  vision_search_image_invalid: 'errorCodes.vision_search_image_invalid',
  vision_search_image_unavailable: 'errorCodes.vision_search_image_unavailable',
  vision_search_timeout: 'errorCodes.vision_search_timeout',
  vision_search_credentials: 'errorCodes.vision_search_credentials',
  vision_search_quota: 'errorCodes.vision_search_quota',
  vision_search_network: 'errorCodes.vision_search_network',
  vision_search_provider_error: 'errorCodes.vision_search_provider_error',
  vision_search_schema_error: 'errorCodes.vision_search_schema_error',
  idempotency_key_required: 'errorCodes.idempotency_key_required',
  idempotency_conflict: 'errorCodes.idempotency_conflict',
  checkout_request_rejected: 'errorCodes.checkout_request_rejected',
  checkout_session_expired: 'errorCodes.checkout_session_expired',
  payment_already_completed: 'errorCodes.payment_already_completed',
  // Account workspace-billing guidance codes (W2 transports): frontend
  // fail-closed errors when no workspace is selected or when the v1 plane
  // has no equivalent surface. Copy lives in account.errors.
  workspace_selection_required: 'account.errors.workspaceRequired',
  workspace_billing_unavailable: 'account.errors.featureUnavailable',
  INVALID_EMAIL_OR_PASSWORD: 'auth.errors.invalidCredentials',
  USER_ALREADY_EXISTS_USE_ANOTHER_EMAIL: 'auth.errors.emailAlreadyRegistered',
  EMAIL_NOT_VERIFIED: 'auth.errors.emailNotVerified',
  TOO_MANY_REQUESTS: 'auth.errors.tooManyRequests',
  INVALID_OTP: 'auth.errors.otpInvalidOrExpired',
  OTP_EXPIRED: 'auth.errors.otpInvalidOrExpired',
}

type ErrorRecord = Record<string, unknown>

function asRecord(value: unknown): ErrorRecord | null {
  return value && typeof value === 'object' ? (value as ErrorRecord) : null
}

function nonNumericCode(value: unknown): string | null {
  return typeof value === 'string' && value.trim() && !/^\d+$/.test(value.trim())
    ? value.trim()
    : null
}

/** Reads stable machine codes only. User-facing prose is never treated as a contract. */
export function userErrorCode(error: unknown): string | null {
  const root = asRecord(error)
  if (!root) return null

  const rootCode = nonNumericCode(root.code)
  if (rootCode) return rootCode

  const data = asRecord(root.data)
  const dataCode = nonNumericCode(data?.error_code) ?? nonNumericCode(data?.code)
  if (dataCode) return dataCode

  const nested = asRecord(root.error)
  const nestedCode = nonNumericCode(nested?.code)
  if (nestedCode) return nestedCode

  return root.cause && root.cause !== error ? userErrorCode(root.cause) : null
}

export function userFacingErrorCopy(error: unknown, fallback: string): string {
  const code = userErrorCode(error)
  const key = code ? USER_ERROR_CODE_COPY_KEY_MAP[code] : undefined
  return key ? i18n.global.t(key) : fallback
}

/** Copy key for a readiness block/error code, or null when the code is unknown. */
export function readinessErrorCodeCopyKey(errorCode?: string): string | null {
  if (!errorCode) return null
  return USER_ERROR_CODE_COPY_KEY_MAP[errorCode] ?? null
}

/** Resolved English copy for a readiness block/error code. */
export function readinessErrorCopy(
  errorCode?: string,
  fallbackKey = 'common.errors.actionUnavailable',
): string {
  const key = readinessErrorCodeCopyKey(errorCode)
  return i18n.global.t(key ?? fallbackKey)
}
