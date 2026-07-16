import type { WorkflowFeature } from '../../../../shared/domain/workflowFeatures'

export interface AdminBillingCoverageItem {
  feature: WorkflowFeature
  model: string
  sku: string
  skuConfigured: boolean
  skuEnabled: boolean
  priceConfigured: boolean
  priceEnabled: boolean
  unitCredits?: number
}
