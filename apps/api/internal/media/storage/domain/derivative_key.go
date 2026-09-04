package domain

import (
	"fmt"
	"path"
	"strings"
)

const ImageDerivativeDirectory = ".ideaboard-derivatives"

func ImageDerivativeObjectKey(sourceKey, assetID, tier, format string) (string, error) {
	normalized, err := NormalizeUploadObjectKey(sourceKey)
	if err != nil {
		return "", err
	}
	if assetID == "" || tier == "" || format == "" {
		return "", fmt.Errorf("图片派生对象参数无效")
	}
	return NormalizeUploadObjectKey(path.Join(
		path.Dir(normalized), ImageDerivativeDirectory, assetID, tier+"."+format,
	))
}

func IsImageDerivativeObjectKey(objectKey string) bool {
	normalized, err := NormalizeUploadObjectKey(objectKey)
	if err != nil {
		return false
	}
	return strings.HasPrefix(normalized, ImageDerivativeDirectory+"/") || strings.Contains(normalized, "/"+ImageDerivativeDirectory+"/")
}
