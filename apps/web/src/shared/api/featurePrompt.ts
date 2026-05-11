import { getAdminFeaturePrompts } from './generated/admin-feature-prompts/admin-feature-prompts'
import type {
  GetAdminFeaturePromptResult,
  ListAdminFeaturePromptsResult,
  SetAdminFeaturePromptResult,
} from './generated/admin-feature-prompts/admin-feature-prompts'

export const featurePromptApi = {
  adminList: (): Promise<ListAdminFeaturePromptsResult> =>
    getAdminFeaturePrompts().listAdminFeaturePrompts(),
  adminGet: (slug: string): Promise<GetAdminFeaturePromptResult> =>
    getAdminFeaturePrompts().getAdminFeaturePrompt(slug),
  adminSet: (slug: string, prompt: string): Promise<SetAdminFeaturePromptResult> =>
    getAdminFeaturePrompts().setAdminFeaturePrompt(slug, { prompt }),
}
