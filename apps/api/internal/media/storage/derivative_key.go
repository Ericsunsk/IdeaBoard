package storage

import (
	"fmt"
	"path"
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
	return normalizeUploadObjectPrefix(path.Join(
		path.Dir(normalized), imageDerivativeDirectory, assetID, tier+"."+format,
	))
}
