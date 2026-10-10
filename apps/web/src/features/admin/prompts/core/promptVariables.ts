import { SERVICE, type Service } from '../../../../core/contracts/imageServices'

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
