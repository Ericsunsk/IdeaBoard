package storage

func (s *StorageService) PreviewURLTransformer() func(string) string {
	return func(raw string) string { return raw }
}

func (s *StorageService) invalidatePreviewURLTransformerCache() {}
