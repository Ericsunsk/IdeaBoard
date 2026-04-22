package imageproc

const SegConfigKey = "aliyun_seg_config"

type AliyunSegConfig struct {
	Enabled         bool   `json:"enabled"`
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	RegionID        string `json:"region_id"`
}

type AliyunSegConfigUpdateRequest struct {
	Enabled         bool    `json:"enabled"`
	AccessKeyID     string  `json:"access_key_id"`
	AccessKeySecret *string `json:"access_key_secret,omitempty"`
	RegionID        string  `json:"region_id"`
}
