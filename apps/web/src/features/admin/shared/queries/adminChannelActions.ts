import channelApi from '../../../../shared/api/channel'
import type { Channel, ChannelFormData, DetectModelsRequest } from '../../../../shared/api/channel'
import { assertApiOk, requireApiData } from './adminApiResponse'

export type { Channel, ChannelFormData }

type SaveAdminChannelInput = {
  id?: number
  data: ChannelFormData
}

export async function fetchAdminChannels(): Promise<Channel[]> {
  const res = await channelApi.list()
  const data = requireApiData(res, '加载渠道商失败')
  return data.list ?? []
}

export async function saveAdminChannel(input: SaveAdminChannelInput): Promise<void> {
  const res = input.id != null
    ? await channelApi.update(input.id, input.data)
    : await channelApi.create(input.data)
  assertApiOk(res, '保存渠道商失败')
}

export async function deleteAdminChannel(id: number): Promise<void> {
  const res = await channelApi.delete(id)
  assertApiOk(res, '删除渠道商失败')
}

export async function detectAdminChannelModels(input: DetectModelsRequest): Promise<string[]> {
  const res = await channelApi.detectModels(input)
  const data = requireApiData(res, '自动检测模型失败')
  return data.models ?? []
}
