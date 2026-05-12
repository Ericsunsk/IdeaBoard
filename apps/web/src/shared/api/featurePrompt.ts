import {
  getAdminFeaturePrompt,
  listAdminFeaturePrompts,
  setAdminFeaturePrompt,
} from './generated-query/admin-feature-prompts/admin-feature-prompts'

export const featurePromptApi = {
  adminList: () => listAdminFeaturePrompts(),
  adminGet: (slug: string) => getAdminFeaturePrompt(slug),
  adminSet: (slug: string, prompt: string) => setAdminFeaturePrompt(slug, { prompt }),
}
