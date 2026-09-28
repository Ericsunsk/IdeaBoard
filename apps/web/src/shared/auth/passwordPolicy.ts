import * as v from 'valibot'

export const PASSWORD_LENGTH_LIMIT = {
  min: 10,
  max: 128,
} as const

export const OTP_CODE_PATTERN = /^\d{6}$/

const EmailSchema = v.pipe(v.string(), v.trim(), v.minLength(1), v.email())

export function passwordSchema(lengthMessage: string) {
  return v.pipe(
    v.string(),
    v.minLength(PASSWORD_LENGTH_LIMIT.min, lengthMessage),
    v.maxLength(PASSWORD_LENGTH_LIMIT.max, lengthMessage),
  )
}

export function emailSchema(invalidMessage: string) {
  return v.pipe(v.string(), v.trim(), v.minLength(1), v.email(invalidMessage))
}

export function otpSchema(invalidMessage: string) {
  return v.pipe(v.string(), v.trim(), v.regex(OTP_CODE_PATTERN, invalidMessage))
}

/** Schema-backed password-length check returning the caller-localized `lengthMessage`. */
export function getPasswordLengthError(password: string, lengthMessage: string) {
  return v.is(passwordSchema(lengthMessage), password) ? '' : lengthMessage
}

/** Schema-backed email check returning `invalidMessage` when the value is not an email. */
export function validateEmail(email: unknown, invalidMessage: string): string {
  return v.is(EmailSchema, email) ? '' : invalidMessage
}

/** Schema-backed 6-digit OTP check (trims first) returning `invalidMessage` on mismatch. */
export function validateOtpCode(code: unknown, invalidMessage: string): string {
  return v.is(otpSchema(invalidMessage), code) ? '' : invalidMessage
}
