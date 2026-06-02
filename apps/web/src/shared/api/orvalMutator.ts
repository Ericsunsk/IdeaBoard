import { api, type ApiClientError } from './client'

export type OrvalRequestConfig = RequestInit

export const orvalMutator = <T>(url: string, options?: OrvalRequestConfig): Promise<T> => {
  return api.request<T>(url, options)
}

export type ErrorType<Error> = Omit<ApiClientError, 'data'> & {
  data?: Error
}

export type BodyType<BodyData> = BodyData
