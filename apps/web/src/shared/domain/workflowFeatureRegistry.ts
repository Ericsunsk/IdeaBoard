import { i18n } from '../i18n'
import { WORKFLOW_FEATURE, type WorkflowFeature } from './workflowFeatures'

export type WorkflowFeaturePromptVariableTone = 'prompt' | 'control' | 'context'

export interface WorkflowFeaturePromptVariable {
  key: `{{${string}}}`
  tone: WorkflowFeaturePromptVariableTone
}

const WORKFLOW_FEATURE_PROMPT_VARIABLES: Readonly<
  Record<WorkflowFeature, readonly WorkflowFeaturePromptVariable[]>
> = {
  [WORKFLOW_FEATURE.render]: [{ key: '{{user_prompt}}', tone: 'prompt' }],
  [WORKFLOW_FEATURE.inpaint]: [],
  [WORKFLOW_FEATURE.variations]: [
    { key: '{{user_prompt}}', tone: 'prompt' },
    { key: '{{variation_strength}}', tone: 'control' },
  ],
  [WORKFLOW_FEATURE.styleFusion]: [
    { key: '{{user_prompt}}', tone: 'prompt' },
    { key: '{{fusion_input_weights}}', tone: 'control' },
  ],
  [WORKFLOW_FEATURE.cutout]: [],
  [WORKFLOW_FEATURE.promptGeneration]: [{ key: '{{user_prompt}}', tone: 'prompt' }],
  [WORKFLOW_FEATURE.hdUpscale]: [],
  [WORKFLOW_FEATURE.outpaint]: [],
}

export function getWorkflowFeaturePromptVariables(
  slug: WorkflowFeature,
): readonly WorkflowFeaturePromptVariable[] {
  return WORKFLOW_FEATURE_PROMPT_VARIABLES[slug]
}

export interface WorkflowFeatureImageActionText {
  label: string
  title: string
}

/** Locale keys mirror slugs (`hd-upscale` -> `canvas.features.hdUpscale`). */
export function workflowFeatureLabelKey(slug: WorkflowFeature): string {
  const camel = slug.replace(/-([a-z])/g, (_, character: string) => character.toUpperCase())
  return `canvas.features.${camel}`
}

export function getWorkflowFeatureImageActionText(
  slug: WorkflowFeature,
): WorkflowFeatureImageActionText {
  // Resolve at read time so a future locale switch does not freeze this copy at module import.
  const label = i18n.global.t(workflowFeatureLabelKey(slug))
  return { label, title: label }
}
