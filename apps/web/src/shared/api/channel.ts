import {
  createAdminChannel,
  deleteAdminChannel,
  detectAdminChannelModels,
  listAdminChannels,
  updateAdminChannel,
} from './generated-query/admin-channels/admin-channels'
export type {
  Channel,
  ChannelFormData,
  DetectModelsRequest,
  DetectModelsResponse,
} from './generated-query/model'

export const channelApi = {
  list: listAdminChannels,
  create: createAdminChannel,
  update: updateAdminChannel,
  delete: deleteAdminChannel,
  detectModels: detectAdminChannelModels,
}

export {
  createAdminChannel as createChannel,
  deleteAdminChannel as deleteChannel,
  detectAdminChannelModels as detectChannelModels,
  listAdminChannels,
  updateAdminChannel as updateChannel,
}

export default channelApi
