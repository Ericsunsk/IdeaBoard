type RuntimeEnvSource = Record<string, string | undefined>

export function isProductionRuntime(source: RuntimeEnvSource = process.env): boolean {
  const value = String(source.NODE_ENV || source.APP_ENV || '')
    .trim()
    .toLowerCase()
  return value === 'production' || value === 'prod' || value === 'release'
}
