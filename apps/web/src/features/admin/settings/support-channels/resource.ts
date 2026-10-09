import {
  getControlSupportChannels,
  updateControlSupportChannels,
} from '../../../../shared/api/generated-query/control/control-platform/control-platform'
import { SupportChannelKind } from '../../../../shared/api/generated-query/control/model/supportChannelKind'
import type { SupportChannelsSetting } from '../../../../shared/api/generated-query/control/model/supportChannelsSetting'
import { requireContractData } from '../../../../shared/api/contractKernel'
import { buildRoutineControlMutationHeaders } from '../../shared/lib/controlMutation'
import { loadControlSetting } from '../../shared/lib/controlResource'
import { withControlCopy } from '../../shared/lib/controlCopy'
export type { SupportChannelsSetting } from '../../../../shared/api/generated-query/control/model/supportChannelsSetting'

export const supportChannelsQueryKey = ['admin', 'settings', 'support-channels'] as const
export const supportChannelKinds = Object.values(SupportChannelKind)
export interface AdminSupportChannelsResource {
  get(): Promise<SupportChannelsSetting>
  update(input: SupportChannelsSetting, operatorReason: string): Promise<SupportChannelsSetting>
}

function toSetting(data: SupportChannelsSetting): SupportChannelsSetting {
  if (!Array.isArray(data.channels)) throw new Error('客户支持响应无效')
  return { channels: data.channels.map((channel) => ({ ...channel })) }
}

async function getSupportChannels(): Promise<SupportChannelsSetting> {
  return loadControlSetting(getControlSupportChannels, {
    fallback: '读取客户支持失败',
    map: toSetting,
  })
}

async function updateSupportChannels(
  input: SupportChannelsSetting,
  operatorReason: string,
): Promise<SupportChannelsSetting> {
  // Routine config write (backend ReasonOptional): the reason may be empty
  // (audit records it as-is) while Idempotency-Key stays required.
  const reason = operatorReason.trim()
  try {
    const data = requireContractData<SupportChannelsSetting>(
      await updateControlSupportChannels(input, {
        headers: buildRoutineControlMutationHeaders({ operatorReason: reason }),
      }),
      '读取客户支持失败',
    )
    return toSetting(data)
  } catch (error) {
    throw withControlCopy(error, '保存客户支持失败')
  }
}

export function createAdminSupportChannelsResource(): AdminSupportChannelsResource {
  return {
    get: getSupportChannels,
    update: updateSupportChannels,
  }
}
