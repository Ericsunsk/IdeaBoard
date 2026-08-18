package storage

import (
	"fmt"
	"path"
	"strconv"
)

const imagePreviewDirectory = ".ideaboard-previews"

func imagePreviewObjectPrefix(sourceKey string) (string, error) {
	normalized, err := normalizeUploadObjectKey(sourceKey)
	if err != nil {
		return "", err
	}
	return normalizeUploadObjectPrefix(path.Join(path.Dir(normalized), imagePreviewDirectory, path.Base(normalized)))
}

func imagePreviewObjectKey(sourceKey string, maxSide int) (string, error) {
	if maxSide <= 0 {
		return "", fmt.Errorf("图片预览尺寸无效")
	}
	prefix, err := imagePreviewObjectPrefix(sourceKey)
	if err != nil {
		return "", err
	}
	return prefix + strconv.Itoa(maxSide), nil
}
