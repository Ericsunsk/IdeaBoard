import {
  listAdminWorkflowFeatures,
  setAdminWorkflowFeature,
} from '../../../shared/api/generated-query/admin-workflow-features/admin-workflow-features'
import type {
  SetAdminWorkflowFeatureBody,
  WorkflowFeature,
} from '../../../shared/api/generated-query/model'
import { resolveApiData } from '../../../shared/api/apiResponse'

export type SaveWorkflowFeatureInput = {
  slug: string
} & SetAdminWorkflowFeatureBody

export async function fetchAdminWorkflowFeatures(): Promise<WorkflowFeature[]> {
  const data = await resolveApiData(listAdminWorkflowFeatures(), '加载工作流配置失败')
  return data.workflow_features ?? []
}

export async function saveAdminWorkflowFeature(
  input: SaveWorkflowFeatureInput,
): Promise<WorkflowFeature> {
  const { slug, ...payload } = input
  return resolveApiData(setAdminWorkflowFeature(slug, payload), '保存工作流配置失败')
}
