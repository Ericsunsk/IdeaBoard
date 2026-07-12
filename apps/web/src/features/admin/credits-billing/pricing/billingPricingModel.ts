import {
  WORKFLOW_FEATURE_REGISTRY_MAP,
  type WorkflowFeatureRegistryItem,
} from '../../../../shared/domain/workflowFeatureRegistry'
import {
  WORKFLOW_FEATURE_SLUGS,
  type WorkflowFeature,
} from '../../../../shared/domain/workflowFeatures'
import type {
  AdminBillingCatalogItem,
  BillingCatalogFilter,
  BillingPriceForm,
  BillingPriceFormErrors,
  BillingSKUForm,
  BillingSKUFormErrors,
} from './billingPricingTypes'
import { validateAdminActionReason } from '../adminActionReasonContract'

const billingSKUPattern = /^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$/
const aiImageSKUPattern = /^ai-image:([a-z][a-z0-9-]*)\.([a-f0-9]{64})$/
const maxCredits = 9_000_000_000_000_000

export interface AIWorkflowSKUIdentity {
  feature: WorkflowFeatureRegistryItem
  productFingerprint: string
}

export function parseAIWorkflowSKU(sku: string): AIWorkflowSKUIdentity | undefined {
  const match = aiImageSKUPattern.exec(sku)
  if (!match) return undefined

  const featureSlug = match[1]
  if (!isWorkflowFeature(featureSlug)) return undefined

  return {
    feature: WORKFLOW_FEATURE_REGISTRY_MAP[featureSlug],
    productFingerprint: match[2],
  }
}

export function isCatalogItemUnpriced(item: AdminBillingCatalogItem): boolean {
  return item.currentPrice == null
}

export function isCatalogItemBillable(item: AdminBillingCatalogItem): boolean {
  return Boolean(item.enabled && item.currentPrice?.enabled)
}

export function filterBillingCatalog(
  items: readonly AdminBillingCatalogItem[],
  searchKeyword: string,
  filter: BillingCatalogFilter,
): AdminBillingCatalogItem[] {
  const keyword = searchKeyword.trim().toLocaleLowerCase()

  return items.filter((item) => {
    if (filter === 'ai-image' && !parseAIWorkflowSKU(item.sku)) return false
    if (filter === 'unpriced' && !isCatalogItemUnpriced(item)) return false
    if (filter === 'disabled' && item.enabled) return false

    if (!keyword) return true
    return [item.sku, item.displayName, item.description].some((value) =>
      value.toLocaleLowerCase().includes(keyword),
    )
  })
}

export function validateBillingSKUForm(
  form: BillingSKUForm,
  isEditing: boolean,
): BillingSKUFormErrors {
  const errors: BillingSKUFormErrors = {}
  const sku = form.sku.trim().toLocaleLowerCase()
  const displayName = form.displayName.trim()

  if (!isEditing && !billingSKUPattern.test(sku)) {
    errors.sku = '格式为 namespace:local-key，仅支持小写字母、数字、点、下划线和连字符'
  }
  if (!displayName) {
    errors.displayName = '请输入展示名称'
  } else if (displayName.length > 120) {
    errors.displayName = '展示名称不能超过 120 个字符'
  }
  if (form.description.length > 1000) {
    errors.description = '说明不能超过 1000 个字符'
  }
  const reasonValidation = validateAdminActionReason(form.reason)
  if (!reasonValidation.valid) errors.reason = reasonValidation.error

  return errors
}

export function validateBillingPriceForm(form: BillingPriceForm): BillingPriceFormErrors {
  const errors: BillingPriceFormErrors = {}
  const value = form.unitCredits
  if (value == null || !Number.isSafeInteger(value) || value < 0 || value > maxCredits) {
    errors.unitCredits = '点子必须是 0 到 9,000,000,000,000,000 的整数'
  }
  const reasonValidation = validateAdminActionReason(form.reason)
  if (!reasonValidation.valid) errors.reason = reasonValidation.error
  return errors
}

export function formatBillingDate(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date)
}

function isWorkflowFeature(value: string): value is WorkflowFeature {
  return (WORKFLOW_FEATURE_SLUGS as readonly string[]).includes(value)
}
