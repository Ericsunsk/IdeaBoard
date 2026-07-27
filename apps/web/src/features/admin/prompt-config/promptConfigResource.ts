import type {
  WorkflowFeature,
  WorkflowFeatureVariable,
  WorkflowFeatureVariableTone,
} from '../../../shared/api/generated-query/model'
import { fetchAdminWorkflowFeatures, saveAdminWorkflowFeature } from './promptConfigApi'

export type { WorkflowFeature, WorkflowFeatureVariable, WorkflowFeatureVariableTone }

export type AdminWorkflowFeatureResource = {
  fetchWorkflowFeatures: typeof fetchAdminWorkflowFeatures
  saveWorkflowFeature: typeof saveAdminWorkflowFeature
}

export function createAdminWorkflowFeatureResource(): AdminWorkflowFeatureResource {
  return {
    fetchWorkflowFeatures: fetchAdminWorkflowFeatures,
    saveWorkflowFeature: saveAdminWorkflowFeature,
  }
}
