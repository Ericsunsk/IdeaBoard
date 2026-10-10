import { readonly, ref } from 'vue'
import { useAsyncAction } from '../../../../core/composables/useAsyncAction'
import {
  getPasswordLengthError,
  validateEmail,
  validateOtpCode,
} from '../../../../core/auth/passwordPolicy'
import { markRequiredError } from '../../../../core/utils/formValidation'

/**
 * Auth fields kit: single owner for email/password/OTP validation outcomes
 * across the login, register, and security surfaces.
 *
 * All checks delegate to `shared/auth/passwordPolicy` (valibot-backed);
 * human copy always arrives as caller-localized messages, never as
 * English-only literals. Required errors keep the required sentinel so
 * `IbFormField` renders them inline as placeholder text.
 */

export const AUTH_DISPLAY_NAME_LIMIT = {
  min: 1,
  max: 100,
} as const

type SentinelOption = {
  /** `false` keeps a plain message (no inline-placeholder rendering). Defaults to `true`. */
  sentinel?: boolean
}

export function requiredFieldError(
  value: string,
  requiredMessage: string,
  options?: SentinelOption,
) {
  if (value) return ''
  return options?.sentinel === false ? requiredMessage : markRequiredError(requiredMessage)
}

export function emailFormatError(value: string, invalidMessage: string) {
  return validateEmail(value, invalidMessage)
}

export function passwordLengthFieldError(value: string, lengthMessage: string) {
  return getPasswordLengthError(value, lengthMessage)
}

/** Single-owner 6-digit OTP check shared by the register, reset, and email-change flows. */
export function otpFieldError(value: string, invalidMessage: string) {
  return validateOtpCode(value, invalidMessage)
}

export function lengthRangeError(
  value: string,
  limit: { min: number; max: number },
  rangeMessage: string,
) {
  if (value.length < limit.min || value.length > limit.max) return rangeMessage
  return ''
}

export function useAuthForm<TField extends string>(fields: readonly TField[]) {
  const initial = Object.fromEntries(fields.map((field) => [field, ''])) as Record<TField, string>
  const values = ref<Record<TField, string>>({ ...initial })
  const errors = ref<Record<TField, string>>({ ...initial })
  const { isPending: isSubmitting, runAsyncAction } = useAsyncAction()

  function validateField(field: TField, validator: (value: string) => string) {
    errors.value[field] = validator(values.value[field])
    return !errors.value[field]
  }

  function clearFieldError(field: TField) {
    if (errors.value[field]) errors.value[field] = ''
  }

  return {
    values,
    errors,
    isSubmitting: readonly(isSubmitting),
    runAsyncAction,
    validateField,
    clearFieldError,
  }
}
