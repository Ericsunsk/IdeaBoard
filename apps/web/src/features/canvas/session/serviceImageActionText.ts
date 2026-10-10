import { i18n } from '../../../core/i18n'
import type { Service } from '../../../core/contracts/imageServices'
import { serviceLabelKey } from '../core/nodes/serviceLabelKey'

export interface ServiceImageActionText {
  label: string
  title: string
}

export function getServiceImageActionText(
  slug: Service,
  resolveLabel: (key: string) => string = (key) => i18n.global.t(key),
): ServiceImageActionText {
  // Resolve at read time so a future locale switch does not freeze this copy at module import.
  // Callers may supply their own resolver; session never hardcodes feature copy itself.
  const label = resolveLabel(serviceLabelKey(slug))
  return { label, title: label }
}
