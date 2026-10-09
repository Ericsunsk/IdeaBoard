import { requireContractData } from '../../../../shared/api/contractKernel'
import {
  buildControlMutationHeaders,
  requireOperatorReason,
  type ControlErrorMessages,
  type ControlMutationOptions,
} from './controlMutation'
import { withControlCopy } from './controlCopy'

// Resources own DTO mapping and whether the operator reason belongs in the body.
export type LoadControlSettingOptions<TApi, TSummary> = {
  fallback: string
  map: (data: TApi) => TSummary
  domainMessages?: ControlErrorMessages | undefined
}

export async function loadControlSetting<TApi, TSummary>(
  fetch: () => Promise<unknown>,
  options: LoadControlSettingOptions<TApi, TSummary>,
): Promise<TSummary> {
  try {
    const data = requireContractData<TApi>(await fetch(), options.fallback)
    return options.map(data)
  } catch (error) {
    throw withControlCopy(error, options.fallback, options.domainMessages)
  }
}

export type StoreControlSettingOptions<TApi, TSummary, TInput> = {
  fallback: string
  map: (data: TApi) => TSummary
  toRequest: (input: TInput, reason: string) => unknown
  persist: (body: unknown, headers: Record<string, string>) => Promise<unknown>
  domainMessages?: ControlErrorMessages | undefined
  dataFallback?: string | undefined
}

export async function storeControlSetting<TApi, TSummary, TInput>(
  input: TInput,
  mutation: ControlMutationOptions,
  options: StoreControlSettingOptions<TApi, TSummary, TInput>,
): Promise<TSummary> {
  const reason = requireOperatorReason(mutation.operatorReason)
  const headers = buildControlMutationHeaders({
    operatorReason: reason,
    idempotencyKey: mutation.idempotencyKey,
  })
  const request = options.toRequest(input, reason)
  try {
    const data = requireContractData<TApi>(
      await options.persist(request, headers),
      options.dataFallback ?? options.fallback,
    )
    return options.map(data)
  } catch (error) {
    throw withControlCopy(error, options.fallback, options.domainMessages)
  }
}

export type ControlSettingResourceConfig<TApi, TSummary, TInput> = {
  getFallback: string
  saveFallback: string
  domainMessages?: ControlErrorMessages | undefined
  saveDataFallback?: string | undefined
  map: (data: TApi) => TSummary
  toRequest: (input: TInput, reason: string) => unknown
  fetch: () => Promise<unknown>
  persist: (body: unknown, headers: Record<string, string>) => Promise<unknown>
}

export type ControlSettingResource<TSummary, TInput> = {
  get: () => Promise<TSummary>
  save: (input: TInput, mutation: ControlMutationOptions) => Promise<TSummary>
}

export function createControlSettingResource<TApi, TSummary, TInput>(
  config: ControlSettingResourceConfig<TApi, TSummary, TInput>,
): ControlSettingResource<TSummary, TInput> {
  return {
    get: () =>
      loadControlSetting(config.fetch, {
        fallback: config.getFallback,
        map: config.map,
        domainMessages: config.domainMessages,
      }),
    save: (input, mutation) =>
      storeControlSetting(input, mutation, {
        fallback: config.saveFallback,
        map: config.map,
        toRequest: config.toRequest,
        persist: config.persist,
        domainMessages: config.domainMessages,
        dataFallback: config.saveDataFallback,
      }),
  }
}
