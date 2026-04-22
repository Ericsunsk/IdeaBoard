package prompts

const FeaturePromptConfigKeyPrefix = "feature_prompt:"

func FeaturePromptConfigKey(slug string) string {
	return FeaturePromptConfigKeyPrefix + slug
}
