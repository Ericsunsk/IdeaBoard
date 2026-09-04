package ports

import (
	"ideaboard/api/internal/media/ingestion"
)

type StorageStoreOptions struct {
	DirPrefix  string
	AllowHosts []string
	ObjectKey  string
}

type StoredUploadRef struct {
	URL            string
	StorageKey     string
	StorageProfile string
	MIMEType       string
	Width          int
	Height         int
	SizeBytes      int64
	ChecksumSHA256 string
	Animated       bool
}

func (r StoredUploadRef) IngestionCompletion(sourceURL string) ingestion.Completion {
	return ingestion.Completion{
		StorageKey:     r.StorageKey,
		URL:            r.URL,
		SourceURL:      sourceURL,
		MIMEType:       r.MIMEType,
		Width:          r.Width,
		Height:         r.Height,
		SizeBytes:      r.SizeBytes,
		ChecksumSHA256: r.ChecksumSHA256,
		Animated:       r.Animated,
	}
}

type StoredUploadObject struct {
	ObjectKey   string
	Filename    string
	ContentType string
	Data        []byte
}

type StoredUploadLookup struct {
	AssetID   int64
	URL       string
	ObjectKey string
}

type StoredDerivativeLookup struct {
	AssetID int64
	Tier    string
}

type StoredUploadLocation struct {
	StorageProfileID string
	StorageKey       string
}
