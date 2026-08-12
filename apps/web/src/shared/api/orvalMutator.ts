import { api, type ApiClientError, type ApiRequestConfig } from './client'

export type OrvalRequestConfig = RequestInit & {
  timeout?: number
}

function toApiRequestConfig(options?: OrvalRequestConfig): ApiRequestConfig | undefined {
  if (!options) return undefined
  const { body, ...rest } = options
  return {
    ...rest,
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
