import type { WorkflowFeature } from '../../../../shared/domain/workflowFeatures'
import {
  WORKFLOW_FEATURE_CONFIG_GROUPS,
  WORKFLOW_FEATURE_CONFIG_ITEM_MAP,
  type WorkflowFeatureConfigGroup,
  type WorkflowFeatureConfigItem,
} from '../../../../shared/domain/workflowFeatureRegistry'

export type FeatureSlug = WorkflowFeature
type FeatureItem = WorkflowFeatureConfigItem

export const featureMap: Record<FeatureSlug, FeatureItem> = WORKFLOW_FEATURE_CONFIG_ITEM_MAP
export const groupedFeatureGroups = WORKFLOW_FEATURE_CONFIG_GROUPS

export type FeatureGroup = WorkflowFeatureConfigGroup
