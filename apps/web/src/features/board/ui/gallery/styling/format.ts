import { formatLocaleDate } from '../../../../../core/i18n/formatters'

export function formatCatalogDate(timestamp: number): string {
  if (!Number.isFinite(timestamp)) return '-'
  return formatLocaleDate(timestamp, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  })
}
