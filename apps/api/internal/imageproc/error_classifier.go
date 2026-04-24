package imageproc

import "strings"

const genericImageProcErrorMessage = "图像处理失败，请稍后重试"

func classifyImageProcError(err error) (int, string) {
	if err == nil {
		return 500, genericImageProcErrorMessage
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "InvalidApi.NotPurchase"):
		return 503, "阿里云超分辨率服务未开通，请联系管理员在阿里云开通图像增强超分能力"
	case strings.Contains(msg, "阿里云视觉AI服务未启用"):
		return 503, "阿里云视觉AI服务未启用，请联系管理员检查配置"
	case strings.Contains(msg, "分割抠图服务未启用"):
		return 503, "分割抠图服务未启用，请联系管理员检查配置"
	default:
		return 500, genericImageProcErrorMessage
	}
}
