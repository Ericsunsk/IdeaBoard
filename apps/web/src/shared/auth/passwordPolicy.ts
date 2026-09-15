import * as v from 'valibot'

export const PASSWORD_LENGTH_LIMIT = {
  min: 10,
  max: 128,
} as const

const PasswordSchema = v.pipe(
  v.string(),
  v.minLength(PASSWORD_LENGTH_LIMIT.min),
  v.maxLength(PASSWORD_LENGTH_LIMIT.max),
)

const EmailSchema = v.pipe(v.string(), v.trim(), v.minLength(1), v.email())

export function getPasswordLengthError(password: string) {
  const { min, max } = PASSWORD_LENGTH_LIMIT
  return v.is(PasswordSchema, password) ? '' : `Password must be ${min}-${max} characters`
}

/** Schema-backed email check returning `invalidMessage` when the value is not an email. */
export function validateEmail(email: unknown, invalidMessage: string): string {
  return v.is(EmailSchema, email) ? '' : invalidMessage
}
