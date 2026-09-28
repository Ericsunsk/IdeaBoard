/**
 * Auth fields kit public entrypoint: field components plus the valibot-backed
 * form wiring shared by the login, register, and security surfaces.
 */
export { default as EmailField } from './EmailField.vue'
export { default as PasswordField } from './PasswordField.vue'
export { default as OtpField } from './OtpField.vue'
export {
  AUTH_DISPLAY_NAME_LIMIT,
  emailFieldError,
  emailFormatError,
  lengthRangeError,
  otpFieldError,
  passwordFieldError,
  passwordLengthFieldError,
  requiredFieldError,
  useAuthForm,
} from './useAuthForm'
export type { AuthFieldMessages } from './useAuthForm'
