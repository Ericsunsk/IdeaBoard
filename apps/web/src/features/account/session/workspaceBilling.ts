import { formatDisplayErrorMessage } from '../../../core/utils/errorMessages'
import { requireWorkspaceId as requireSharedWorkspaceId } from '../../../core/utils/workspaceId'
import { parseContractEnvelope, requireContractData } from '../../../api/contractKernel'
import { i18n } from '../../../core/i18n'

// Workspace billing seam for the account surface (W2 transports).
//
// User billing moved to the Public Data Plane (`/api/v1/workspaces/{id}/…`):
// every account read carries the active workspace id resolved from the
// existing workspace selection store (personal workspace default). There is
// no hidden inference: a null selection fails closed with a guidance error
// (`workspace_selection_required`) instead of guessing a workspace.
// Surfaces with no v1 equivalent (rewards, purchase options, pending
// orders, recharge history) fail closed with `workspace_billing_unavailable`.
export const WORKSPACE_SELECTION_REQUIRED_CODE = 'workspace_selection_required'
const WORKSPACE_BILLING_UNAVAILABLE_CODE = 'workspace_billing_unavailable'

export type WorkspaceBillingGuidanceError = Error & { code: string }

function guidanceError(code: string, message: string): WorkspaceBillingGuidanceError {
  const error = new Error(message) as WorkspaceBillingGuidanceError
  error.code = code
  return error
}

export function requireAccountWorkspaceId(workspaceId: string | null | undefined): string {
  return requireSharedWorkspaceId(workspaceId, () =>
    guidanceError(
      WORKSPACE_SELECTION_REQUIRED_CODE,
      i18n.global.t('account.errors.workspaceRequired'),
    ),
  )
}

/**
 * Account-owned error-code copy keys (issue #835). The shared error catalog
 * stays feature-free; account surfaces pass this overlay to
 * `userFacingErrorCopy` so frontend guidance errors keep their copy.
 */
export const ACCOUNT_GUIDANCE_COPY_KEYS: Readonly<Record<string, string>> = {
  workspace_selection_required: 'account.errors.workspaceRequired',
  workspace_billing_unavailable: 'account.errors.featureUnavailable',
}

export async function resolveWorkspaceBillingData<T>(
  request: Promise<unknown>,
  fallback: string,
): Promise<T> {
  try {
    return requireContractData<T>(await request, fallback)
  } catch (error) {
    throw contextualizeWorkspaceBillingError(error, fallback)
  }
}

// Paginated list bodies already carry the contract `{data, meta}` shape, so
// callers read the whole body instead of unwrapping it. Contract error
// envelopes still throw with their safe message.
export function readContractListBody<T>(body: unknown, fallback: string): T {
  const parsed = parseContractEnvelope<unknown>(body)
  if (!parsed.ok) {
    const error = new Error(parsed.error.message || fallback) as Error & { code?: string }
    error.code = parsed.error.code
    throw asWorkspaceBillingError(error, fallback)
  }
  return body as T
}

function contextualizeWorkspaceBillingError(error: unknown, fallback: string): Error {
  if (isGuidanceError(error)) return error
  return asWorkspaceBillingError(error, fallback)
}

export function asWorkspaceBillingError(error: unknown, fallback: string): Error {
  const message = formatDisplayErrorMessage(error, fallback)
  if (error instanceof Error && error.message === message) return error
  const contextual = new Error(message, { cause: error }) as Error & {
    code?: unknown
    status?: unknown
  }
  if (error && typeof error === 'object') {
    const source = error as { code?: unknown; status?: unknown }
    contextual.code = source.code
    contextual.status = source.status
  }
  return contextual
}

function isGuidanceError(error: unknown): error is WorkspaceBillingGuidanceError {
  return (
    error instanceof Error &&
    'code' in error &&
    (error.code === WORKSPACE_SELECTION_REQUIRED_CODE ||
      error.code === WORKSPACE_BILLING_UNAVAILABLE_CODE)
  )
}
