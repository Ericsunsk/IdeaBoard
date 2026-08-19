package storage

import (
	"fmt"
	"path"
	"strings"
)

const imageDerivativeDirectory = ".ideaboard-derivatives"

func ImageDerivativeObjectKey(sourceKey, assetID, tier, format string) (string, error) {
	normalized, err := normalizeUploadObjectKey(sourceKey)
	if err != nil {
		return "", err
	}
	if assetID == "" || tier == "" || format == "" {
		return "", fmt.Errorf("图片派生对象参数无效")
	}
	return normalizeUploadObjectKey(path.Join(
		path.Dir(normalized), imageDerivativeDirectory, assetID, tier+"."+format,
	))
}

func IsImageDerivativeObjectKey(objectKey string) bool {
	normalized, err := normalizeUploadObjectKey(objectKey)
	if err != nil {
		return false
	}
	return strings.HasPrefix(normalized, imageDerivativeDirectory+"/") || strings.Contains(normalized, "/"+imageDerivativeDirectory+"/")
}
