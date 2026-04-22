package constants

import "strconv"

const (
	TokenRedisKeyPrefix          = "token:"
	FeaturePromptConfigKeyPrefix = "feature_prompt:"
	StorageProfilesConfigKey     = "storage_profiles_v1"
	AliyunSegConfigKey           = "aliyun_seg_config"
)

func TokenRedisKey(userID uint64) string {
	return TokenRedisKeyPrefix + strconv.FormatUint(userID, 10)
}

func FeaturePromptConfigKey(slug string) string {
	return FeaturePromptConfigKeyPrefix + slug
}
