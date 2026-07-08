import type { SubmitTaskData } from '../../../../shared/api/task'
import { WORKFLOW_FEATURE } from '../../../../shared/domain/workflowFeatures'
import {
  PROMPT_ASPECT_RATIO_OPTIONS,
  type PromptAspectRatio,
} from '../../../canvas-board/domain'

const PATTERN_GEN_ALLOWED_PARAM_KEYS = new Set(['feature', 'model', 'ratio'])
const PATTERN_GEN_MEDIA_PARAM_KEYS = new Set([
  'image_urls',
  'original_image_url',
  'mask_image_url',
  'reference_image_urls',
  'use_mask',
])
const PATTERN_GEN_RATIO_OPTIONS = new Set<string>(PROMPT_ASPECT_RATIO_OPTIONS)
const PATTERN_GEN_RATIO_LABEL = PROMPT_ASPECT_RATIO_OPTIONS.join('、')

export type PatternGenInputContract = {
  prompt: string
  ratio?: PromptAspectRatio
}

export type PatternGenOutputContract = {
  kind: 'image-results'
  minCount: 1
}

export const PATTERN_GEN_NODE_IO_CONTRACT = {
  feature: WORKFLOW_FEATURE.patternGen,
  input: {
    prompt: { required: true },
    ratio: PROMPT_ASPECT_RATIO_OPTIONS,
  },
  output: {
    kind: 'image-results',
    minCount: 1,
  },
} as const

export class PatternGenContractError extends Error {
  constructor(message: string) {
    super(`图案生成参数错误：${message}`)
    this.name = 'PatternGenContractError'
  }
}

function hasValues(values: unknown): boolean {
  return Array.isArray(values) && values.length > 0
}

function assertPatternGenParam(key: string, value: unknown) {
  if (!PATTERN_GEN_ALLOWED_PARAM_KEYS.has(key)) {
    if (PATTERN_GEN_MEDIA_PARAM_KEYS.has(key)) {
      throw new PatternGenContractError('图案生成不需要输入图片，请移除图片输入后重试')
    }
    throw new PatternGenContractError(`图案生成暂不支持参数 ${key}`)
  }
  if (key === 'feature' && `${value ?? ''}`.trim() !== WORKFLOW_FEATURE.patternGen) {
    throw new PatternGenContractError('图案生成任务必须使用 pattern-gen workflow')
  }
  if (key === 'model' && value != null && typeof value !== 'string') {
    throw new PatternGenContractError('图案生成参数 model 必须是字符串')
  }
  if (key === 'ratio') {
    const ratio = `${value ?? ''}`.trim()
    if (!PATTERN_GEN_RATIO_OPTIONS.has(ratio)) {
      throw new PatternGenContractError(
        `图案生成尺寸比例仅支持 ${PATTERN_GEN_RATIO_LABEL}`,
      )
    }
  }
}

export function assertPatternGenTaskRequest(request: SubmitTaskData): void {
  if (`${request.feature ?? ''}`.trim() !== WORKFLOW_FEATURE.patternGen) {
    throw new PatternGenContractError('图案生成任务必须使用 pattern-gen workflow')
  }
  if (!`${request.prompt ?? ''}`.trim()) {
    throw new PatternGenContractError('请输入 Prompt 后再生成图案')
  }
  if (hasValues(request.image_urls)) {
    throw new PatternGenContractError('图案生成不需要输入图片，请移除图片输入后重试')
  }

  const params = request.params as Record<string, unknown> | undefined
  for (const [key, value] of Object.entries(params ?? {})) {
    assertPatternGenParam(key, value)
  }
}
