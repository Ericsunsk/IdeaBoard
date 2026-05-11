import type { AxiosError, AxiosRequestConfig } from 'axios'
import { api } from './client'

export const orvalMutator = <T>(
  config: AxiosRequestConfig,
  options?: AxiosRequestConfig,
): Promise<T> => {
  const headers = {
    ...config.headers,
    ...options?.headers,
  }

  return api.request<unknown, T>({
    ...config,
    ...options,
    headers,
  })
}

export type ErrorType<Error> = AxiosError<Error>
export type BodyType<BodyData> = BodyData
