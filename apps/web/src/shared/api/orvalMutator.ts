import { api, type ApiClientError, type ApiRequestConfig } from './client'

export type OrvalRequestConfig = Omit<RequestInit, 'signal'> & {
  timeout?: number
  // Resources forward optional AbortSignals; undefined is normalized to
  // absent in toApiRequestConfig so fetch semantics never change.
  signal?: AbortSignal | null | undefined
}

function toApiRequestConfig(options?: OrvalRequestConfig): ApiRequestConfig | undefined {
  if (!options) return undefined
  const { body, signal, ...rest } = options
  return {
    ...rest,
    ...(signal == null ? {} : { signal }),
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
