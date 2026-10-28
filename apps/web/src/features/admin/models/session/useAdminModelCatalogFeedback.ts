import { defineFeedback } from '../../shared'

export type AdminModelCatalogFeedback = {
  catalogSynced: () => void
  syncFailed: (error: unknown) => void
  listFailed: (error: unknown) => void
  importSkippedAsExisting: () => void
  imported: (count: number) => void
  addFailed: (error: unknown) => void
  detectFailed: (error: unknown) => void
  deleted: (modelName: string) => void
  deleteFailed: (error: unknown) => void
  duplicateId: (modelId: string) => void
  saved: (isEditing: boolean) => void
  saveFailed: (error: unknown) => void
  clipboardUnsupported: () => void
  copyFailed: () => void
}

export type AdminGatewayConfigFeedback = {
  testPassed: (message: string | null | undefined) => void
  testFailed: (message: string | null | undefined) => void
  loadFailed: (error: unknown) => void
  saveFailed: (error: unknown) => void
  rereadFailed: (error: unknown) => void
  saved: () => void
}

const ADMIN_MODEL_CATALOG_COPY = {
  catalogSynced: { success: '模型库已更新至最新' },
  syncFailed: { error: '同步模型库失败' },
  listFailed: { error: '加载模型目录失败' },
  importSkippedAsExisting: { info: '所选模型已全部存在于模型库中' },
  imported: { success: (count: number) => `已成功导入 ${count} 个模型到模型库` },
  addFailed: { error: '添加模型失败' },
  detectFailed: { error: '探测网关模型失败' },
  deleted: { success: (modelName: string) => `已删除模型「${modelName}」` },
  deleteFailed: { error: '删除模型失败' },
  duplicateId: { errorText: (modelId: string) => `模型标识 ${modelId} 已存在，不能重复添加` },
  saved: { success: (isEditing: boolean) => (isEditing ? '模型修改已保存' : '已成功新增模型') },
  saveFailed: { error: '保存模型失败' },
  clipboardUnsupported: { errorText: '当前环境不支持剪贴板复制' },
  copyFailed: { errorText: '复制失败，请手动复制模型标识' },
}

const ADMIN_GATEWAY_CONFIG_COPY = {
  testPassed: { successMessage: '网关连接测试成功' },
  testFailed: { errorMessage: '网关连接测试失败' },
  loadFailed: { error: '读取网关配置失败' },
  saveFailed: { error: '保存网关配置失败' },
  rereadFailed: { error: '配置已保存，但重读网关状态失败，请重试' },
  saved: { success: 'AI 网关配置已保存' },
}

export function useAdminModelCatalogFeedback(): AdminModelCatalogFeedback {
  return defineFeedback<AdminModelCatalogFeedback>(ADMIN_MODEL_CATALOG_COPY)
}

export function useAdminGatewayConfigFeedback(): AdminGatewayConfigFeedback {
  return defineFeedback<AdminGatewayConfigFeedback>(ADMIN_GATEWAY_CONFIG_COPY)
}
