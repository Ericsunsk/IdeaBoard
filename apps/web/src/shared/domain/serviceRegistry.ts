import { i18n } from '../i18n'
import { SERVICE, type Service } from './imageServices'

export type ServicePromptVariableTone = 'prompt' | 'control' | 'context'

export interface ServicePromptVariable {
  key: `{{${string}}}`
  tone: ServicePromptVariableTone
}

const SERVICE_PROMPT_VARIABLES: Readonly<Record<Service, readonly ServicePromptVariable[]>> = {
  [SERVICE.render]: [{ key: '{{user_prompt}}', tone: 'prompt' }],
  [SERVICE.inpaint]: [],
  [SERVICE.variations]: [
    { key: '{{user_prompt}}', tone: 'prompt' },
    { key: '{{variation_strength}}', tone: 'control' },
  ],
  [SERVICE.fusion]: [
    { key: '{{user_prompt}}', tone: 'prompt' },
    { key: '{{fusion_input_weights}}', tone: 'control' },
  ],
  [SERVICE.cutout]: [],
  [SERVICE.generation]: [{ key: '{{user_prompt}}', tone: 'prompt' }],
  [SERVICE.upscale]: [],
  [SERVICE.outpaint]: [],
}

export function getServicePromptVariables(slug: Service): readonly ServicePromptVariable[] {
  return SERVICE_PROMPT_VARIABLES[slug]
}

export interface ServiceImageActionText {
  label: string
  title: string
}

/** Locale keys mirror slugs (`upscale` -> `canvas.features.upscale`). */
export function serviceLabelKey(slug: Service): string {
  const camel = slug.replace(/-([a-z])/g, (_, character: string) => character.toUpperCase())
  return `canvas.features.${camel}`
}

export function getServiceImageActionText(
  slug: Service,
  resolveLabel: (key: string) => string = (key) => i18n.global.t(key),
): ServiceImageActionText {
  // Resolve at read time so a future locale switch does not freeze this copy at module import.
  // Callers may supply their own resolver; shared never hardcodes feature copy itself.
  const label = resolveLabel(serviceLabelKey(slug))
  return { label, title: label }
}
