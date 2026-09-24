import { formatDisplayErrorMessage } from '../utils/errorMessages'

// Legacy `/api/user/*` response boundary (issue #809).
//
// The backend user plane still serves the shared `{code, msg, data}`
// envelope (`apps/api/internal/platform/response`), so this boundary is
// restored decoupled from generated code: the envelope is declared locally
// instead of importing the deleted `generated-query/model` types. New
// Public/Control-plane code must use `contractKernel` instead; only the
// surviving `/api/user/*` transports import this module.
export type ApiResponseBase = {
  code: number
  msg: string
  message_safe?: boolean | undefined
}

export type ApiResponseWithData<T> = ApiResponseBase & {
  data?: T | null | undefined
}

export function requireApiData<T>(response: ApiResponseWithData<T>, fallback: string): T {
  if (response.code !== 0) {
    throw new Error(response.message_safe ? response.msg : fallback)
  }
  if (response.data == null) throw new Error(fallback)
  return response.data
}

export function createApiDataSelector<T>(fallback: string) {
  return (response: ApiResponseWithData<T>): T => requireApiData(response, fallback)
}

export function assertApiOk(response: ApiResponseBase, fallback: string): void {
  if (response.code !== 0) {
    throw new Error(response.message_safe ? response.msg : fallback)
  }
}

export async function resolveApiData<T>(
  request: Promise<ApiResponseWithData<T>>,
  fallback: string,
): Promise<T> {
  try {
    return requireApiData(await request, fallback)
  } catch (error) {
    throw contextualizeApiError(error, fallback)
  }
}

export async function resolveApiOk(
  request: Promise<ApiResponseBase>,
  fallback: string,
): Promise<void> {
  try {
    assertApiOk(await request, fallback)
  } catch (error) {
    throw contextualizeApiError(error, fallback)
  }
}

function contextualizeApiError(error: unknown, fallback: string): Error {
  const message = formatDisplayErrorMessage(error, fallback)
  if (error instanceof Error && error.message === message) return error

  const contextualError = new Error(message, { cause: error }) as Error & {
    code?: unknown
    data?: unknown
    msg?: string
    status?: unknown
  }
  if (error && typeof error === 'object') {
    const source = error as { code?: unknown; data?: unknown; status?: unknown }
    contextualError.code = source.code
    contextualError.data = source.data
    contextualError.status = source.status
  }
  contextualError.msg = message
  return contextualError
}
