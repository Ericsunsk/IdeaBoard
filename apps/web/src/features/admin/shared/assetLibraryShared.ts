import { extractSafeApiErrorMessage as extractApiErrorMessage } from '../../../shared/utils/errorMessages'
export { extractApiErrorMessage }

export const ADMIN_STATUS_OPTIONS = [
  { label: '已上架', value: 1 },
  { label: '已下架', value: 0 },
]

type ListResponse<T> = { code?: number; data?: { list?: T[] } | null }

export function getApiList<T>(response: ListResponse<T> | null | undefined): T[] {
  return response?.data?.list || []
}

export function splitCommaValues(input: string | null | undefined): string[] {
  if (!input) return []
  return input
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

/**
 * Get the CSS class/variant for a model based on its family (GPT, MJ, SD, etc.)
 */
export function getModelCategoryClass(model: string): string {
  const m = model.toLowerCase()
  if (m.includes('dalle') || m.includes('gpt')) return 'is-indigo'
  if (m.includes('mj') || m.includes('midjourney')) return 'is-purple'
  if (m.includes('sd') || m.includes('stable')) return 'is-cyan'
  return ''
}
