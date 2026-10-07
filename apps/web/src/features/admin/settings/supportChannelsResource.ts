import {
  getControlSupportChannels,
  updateControlSupportChannels,
} from '../../../shared/api/generated-query/control/control-platform/control-platform'
import { requireContractData } from '../../../shared/api/contractKernel'
import { buildControlMutationHeaders, requireOperatorReason } from '../shared/lib/controlMutation'
import { withControlCopy } from '../shared/lib/controlCopy'
import { SupportChannelKind } from '../../../shared/api/generated-query/control/model/supportChannelKind'
import type { SupportChannelsSetting } from '../../../shared/api/generated-query/control/model/supportChannelsSetting'
export type { SupportChannelsSetting } from '../../../shared/api/generated-query/control/model/supportChannelsSetting'

export const supportChannelsQueryKey = ['admin', 'settings', 'support-channels'] as const
export const supportChannelKinds = Object.values(SupportChannelKind)
export interface AdminSupportChannelsResource {
  get(): Promise<SupportChannelsSetting>
  update(input: SupportChannelsSetting, operatorReason: string): Promise<SupportChannelsSetting>
}

function readSetting(body: unknown): SupportChannelsSetting {
  const data = requireContractData<SupportChannelsSetting>(body, '读取客服渠道失败')
  if (!Array.isArray(data.channels)) throw new Error('客服渠道响应无效')
  return { channels: data.channels.map((channel) => ({ ...channel })) }
}

export function createAdminSupportChannelsResource(): AdminSupportChannelsResource {
  return {
    async get() {
      try {
        return readSetting(await getControlSupportChannels())
      } catch (error) {
        throw withControlCopy(error, '读取客服渠道失败')
      }
    },
    async update(input, operatorReason) {
      const reason = requireOperatorReason(operatorReason)
      try {
        return readSetting(
          await updateControlSupportChannels(input, {
            headers: buildControlMutationHeaders({ operatorReason: reason }),
          }),
        )
      } catch (error) {
        throw withControlCopy(error, '保存客服渠道失败')
      }
    },
  }
}
