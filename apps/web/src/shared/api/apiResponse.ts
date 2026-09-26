import { formatDisplayErrorMessage } from '../utils/errorMessages'

// Shared `{code, msg, data}` response boundary (issue #809).
//
// Served by the surviving hand-rolled Public transports (`/api/settings/*`,
// `/api/search`, ...). Control Plane routes use the unified contract
// envelope (`{ data }` / `{ error: { code, message, request_id } }`;
// only the binary brand-logo and SSE monitor stream stay raw), so no
// Control caller imports this module. The envelope is declared locally
// instead of importing generated types. New Public/Control-plane code must
// use `contractKernel` instead; only the transports above import this
// module.
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
