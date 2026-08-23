import { markRequiredError } from '../../../shared/utils/formValidation'

export const AUTH_DISPLAY_NAME_LIMIT = {
  min: 1,
  max: 100,
} as const

export function getRequiredError(value: string, label: string) {
  return value ? '' : markRequiredError(`${label} is required`)
}

export function getLengthError(value: string, limit: { min: number; max: number }, label: string) {
  if (value.length < limit.min || value.length > limit.max) {
    return `${label} must be between ${limit.min} and ${limit.max} characters`
  }
  return ''
}
