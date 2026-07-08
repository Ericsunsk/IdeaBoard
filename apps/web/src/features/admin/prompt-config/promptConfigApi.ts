import {
  evaluateAdminFeaturePrompt,
  getLatestAdminFeaturePromptEvalReport,
  listAdminFeaturePrompts,
  setAdminFeaturePrompt,
} from '../../../shared/api/generated-query/admin-feature-prompts/admin-feature-prompts'
import type {
  EvaluateAdminFeaturePromptBody,
  FeaturePrompt,
  FeaturePromptEvalReport,
  SetAdminFeaturePromptBody,
} from '../../../shared/api/generated-query/model'
import { assertApiOk, requireApiData } from '../shared/queries/adminApiResponse'

export type SaveFeaturePromptInput = {
  slug: string
} & SetAdminFeaturePromptBody

export async function fetchAdminFeaturePrompts(): Promise<FeaturePrompt[]> {
  const res = await listAdminFeaturePrompts()
  const data = requireApiData(res, '加载提示词失败')
  return data.prompts ?? []
}

export async function publishAdminFeaturePrompt(
  input: SaveFeaturePromptInput,
): Promise<FeaturePrompt> {
  const { slug, ...payload } = input
  const res = await setAdminFeaturePrompt(slug, payload)
  return requireApiData(res, '保存提示词失败')
}

export type EvaluateFeaturePromptInput = {
  slug: string
} & EvaluateAdminFeaturePromptBody

export async function evaluateAdminFeaturePromptDraft(
  input: EvaluateFeaturePromptInput,
): Promise<FeaturePromptEvalReport> {
  const { slug, ...payload } = input
  const res = await evaluateAdminFeaturePrompt(slug, payload)
  const report = requireApiData(res, '运行 Prompt eval 失败')
  if (report == null) {
    throw new Error('运行 Prompt eval 失败')
  }
  return report
}

export async function fetchLatestAdminFeaturePromptEvalReport(
  slug: string,
): Promise<FeaturePromptEvalReport | null> {
  const res = await getLatestAdminFeaturePromptEvalReport(slug)
  assertApiOk(res, '加载 Prompt eval 结果失败')
  return res.data ?? null
}
