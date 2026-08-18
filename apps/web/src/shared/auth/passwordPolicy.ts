export const PASSWORD_LENGTH_LIMIT = {
  min: 10,
  max: 128,
} as const

export function getPasswordLengthError(password: string) {
  const { min, max } = PASSWORD_LENGTH_LIMIT
  return password.length < min || password.length > max ? `密码长度需为 ${min}-${max} 个字符` : ''
}
