import { requireContractData } from '@/api/http/contractKernel'
import type { ControlErrorMessages } from '../core/controlCopy'
import { withControlCopy } from '../core/controlCopy'

// Resources own DTO mapping and load error copy.
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
