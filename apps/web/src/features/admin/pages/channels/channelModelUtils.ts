import { extractSafeApiErrorMessage as extractApiErrorMessage } from '../../../../shared/utils/errorMessages'

export { extractApiErrorMessage }

export function splitCommaValues(input: string | null | undefined): string[] {
  if (!input) return []
  return input
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

export function getModelCategoryClass(model: string): string {
  const m = model.toLowerCase()
  if (m.includes('dalle') || m.includes('gpt')) return 'is-indigo'
  if (m.includes('mj') || m.includes('midjourney')) return 'is-purple'
  if (m.includes('sd') || m.includes('stable')) return 'is-cyan'
  return ''
}

