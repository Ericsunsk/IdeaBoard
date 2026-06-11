export function isRequiredErrorMessage(error: string) {
  return error.startsWith('请输入') || error.startsWith('请确认')
}
