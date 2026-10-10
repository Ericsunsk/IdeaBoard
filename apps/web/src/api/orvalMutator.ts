import { api, type ApiClientError, type ApiRequestConfig, type AuthFailureMode } from './client'

type OrvalRequestConfig = Omit<RequestInit, 'signal'> & {
  timeout?: number
  // Resources forward optional AbortSignals; undefined is normalized to
  // absent in toApiRequestConfig so fetch semantics never change.
  signal?: AbortSignal | null | undefined
  // Per-request 401 policy, forwarded to `api.request`: `silent` clears
  // local auth state without navigating (supplementary reads only).
  authFailure?: AuthFailureMode | undefined
}

function toApiRequestConfig(options?: OrvalRequestConfig): ApiRequestConfig | undefined {
  if (!options) return undefined
  const { body, signal, authFailure, ...rest } = options
  return {
    ...rest,
    ...(signal == null ? {} : { signal }),
    ...(authFailure == null ? {} : { authFailure }),
    ...(body == null ? {} : { body: body as ApiRequestConfig['body'] }),
  }
}

export const orvalMutator = <T>(url: string, options?: OrvalRequestConfig): Promise<T> => {
  return api.request<T>(url, toApiRequestConfig(options))
}

export type ErrorType<Error> = Omit<ApiClientError, 'data'> & {
  data?: Error
}

export type BodyType<BodyData> = BodyData
