import type { WorkflowFeature } from '../../../../shared/domain/workflowFeatures'
import {
  WORKFLOW_FEATURE_PROMPT_CONFIG_GROUPS,
  WORKFLOW_FEATURE_PROMPT_CONFIG_ITEM_MAP,
  type WorkflowFeaturePromptConfigGroup,
  type WorkflowFeaturePromptConfigItem,
} from '../../../../shared/domain/workflowFeatureRegistry'

export type FeatureSlug = WorkflowFeature
type FeatureItem = WorkflowFeaturePromptConfigItem

export const featureMap: Record<FeatureSlug, FeatureItem> = WORKFLOW_FEATURE_PROMPT_CONFIG_ITEM_MAP
export const groupedFeatureGroups = WORKFLOW_FEATURE_PROMPT_CONFIG_GROUPS

export type FeatureGroup = WorkflowFeaturePromptConfigGroup

export function getGroupFilledCount(
  group: FeatureGroup,
  savedPromptMap: Record<string, string>,
): number {
  return group.items.filter((item) => (savedPromptMap[item.slug] || '').trim().length > 0).length
}
