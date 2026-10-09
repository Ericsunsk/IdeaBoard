import {
  getControlSupportChannels,
  updateControlSupportChannels,
} from '../../../../shared/api/generated-query/control/control-platform/control-platform'
import { SupportChannelKind } from '../../../../shared/api/generated-query/control/model/supportChannelKind'
import type { SupportChannelsSetting } from '../../../../shared/api/generated-query/control/model/supportChannelsSetting'
import { createControlSettingResource } from '../../shared/lib/controlResource'
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

const settingResource = createControlSettingResource({
  getFallback: '读取客户支持失败',
  saveFallback: '保存客户支持失败',
  // The save path keeps the legacy shared-reader unpack string for
  // malformed envelopes; only the transport copy converged.
  saveDataFallback: '读取客户支持失败',
  map: toSetting,
  toRequest: (input: SupportChannelsSetting) => input,
  fetch: () => getControlSupportChannels(),
  persist: (body, headers) =>
    updateControlSupportChannels(body as SupportChannelsSetting, { headers }),
})

export function createAdminSupportChannelsResource(): AdminSupportChannelsResource {
  return {
    get: () => settingResource.get(),
    update: (input, operatorReason) => settingResource.save(input, { operatorReason }),
  }
}
