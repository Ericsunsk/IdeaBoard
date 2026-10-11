import { formatAdminDisplayError, type AdminToastPort, useAdminToast } from '../../shared'

// 反馈公约（admin，见 settings useSettingsFeedback 文件头）：领域文案收口到本文件
// 的 feedback 工厂，调用方只表达意图，不直接拼 toast 文案。

export type AdminModelCatalogFeedback = {
  catalogSynced: () => void
  syncFailed: (error: unknown) => void
  importSkippedAsExisting: () => void
  imported: (count: number) => void
  addFailed: (error: unknown) => void
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
  saved: () => void
}

function createAdminModelCatalogFeedback(toast: AdminToastPort): AdminModelCatalogFeedback {
  return {
    catalogSynced: () => toast.success('模型库已更新至最新'),
    syncFailed: (error) => toast.error(formatAdminDisplayError(error, '同步模型库失败')),
    importSkippedAsExisting: () => toast.info('所选模型已全部存在于模型库中'),
    imported: (count) => toast.success(`已成功导入 ${count} 个模型到模型库`),
    addFailed: (error) => toast.error(formatAdminDisplayError(error, '添加模型失败')),
    deleted: (modelName) => toast.success(`已删除模型「${modelName}」`),
    deleteFailed: (error) => toast.error(formatAdminDisplayError(error, '删除模型失败')),
    duplicateId: (modelId) => toast.error(`模型标识 ${modelId} 已存在，不能重复添加`),
    saved: (isEditing) => toast.success(isEditing ? '模型修改已保存' : '已成功新增模型'),
    saveFailed: (error) => toast.error(formatAdminDisplayError(error, '保存模型失败')),
    clipboardUnsupported: () => toast.error('当前环境不支持剪贴板复制'),
    copyFailed: () => toast.error('复制失败，请手动复制模型标识'),
  }
}

function createAdminGatewayConfigFeedback(toast: AdminToastPort): AdminGatewayConfigFeedback {
  return {
    testPassed: (message) => toast.success(message || '网关连接测试成功'),
    testFailed: (message) => toast.error(message || '网关连接测试失败'),
    saved: () => toast.success('AI 网关配置已保存'),
  }
}

export function useAdminModelCatalogFeedback() {
  return createAdminModelCatalogFeedback(useAdminToast())
}

export function useAdminGatewayConfigFeedback() {
  return createAdminGatewayConfigFeedback(useAdminToast())
}
