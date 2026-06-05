import { extractSafeApiErrorMessage as extractApiErrorMessage } from '../../../../shared/utils/errorMessages'

export { extractApiErrorMessage }

export function splitCommaValues(input: string | null | undefined): string[] {
  if (!input) return []
  return input
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}
