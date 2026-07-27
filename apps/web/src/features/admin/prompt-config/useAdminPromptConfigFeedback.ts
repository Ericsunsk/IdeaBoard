import {
  formatAdminDisplayError,
  type AdminToastPort,
  useAdminToast,
} from '../shared/lib/adminToast'

export type AdminWorkflowFeatureFeedback = {
  loadWorkflowFeaturesFailed: (error: unknown) => void
  workflowFeatureDraftInvalid: (message: string) => void
  workflowFeatureSaved: (featureTitle: string) => void
  saveWorkflowFeatureFailed: (error: unknown) => void
}

export function createAdminWorkflowFeatureFeedback(
  toast: AdminToastPort,
): AdminWorkflowFeatureFeedback {
  return {
    loadWorkflowFeaturesFailed: (error) =>
      toast.error(formatAdminDisplayError(error, '加载工作流配置失败')),
    workflowFeatureDraftInvalid: (message) => toast.warning(message),
    workflowFeatureSaved: (featureTitle) => toast.success(`「${featureTitle}」配置已保存`),
    saveWorkflowFeatureFailed: (error) =>
      toast.error(formatAdminDisplayError(error, '保存工作流配置失败')),
  }
}

export function useAdminWorkflowFeatureFeedback() {
  return createAdminWorkflowFeatureFeedback(useAdminToast())
}
