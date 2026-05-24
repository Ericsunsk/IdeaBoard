export function isProductionRuntime(): boolean {
  const value = String(process.env.NODE_ENV || process.env.APP_ENV || '').trim().toLowerCase()
  return value === 'production' || value === 'prod' || value === 'release'
}
