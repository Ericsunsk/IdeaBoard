package ingestion

type Origin string

const (
	OriginBoardUpload            Origin = "board_upload"
	OriginGenerationPreparation  Origin = "generation_prepare"
	OriginTaskResult             Origin = "task_result"
	OriginLibraryMaterialization Origin = "library_materialization"
)

type OriginFacts interface {
	IngestionOrigin() Origin
}

type BoardUploadFacts struct {
	BoardID string
}

func (BoardUploadFacts) IngestionOrigin() Origin { return OriginBoardUpload }

type GenerationPreparationFacts struct {
	RefID      string
	Role       string
	InputIndex int
}

func (GenerationPreparationFacts) IngestionOrigin() Origin {
	return OriginGenerationPreparation
}

type TaskResultFacts struct {
	TaskID      string
	ResultIndex int
}

func (TaskResultFacts) IngestionOrigin() Origin { return OriginTaskResult }

type LibraryMaterializationFacts struct {
	LibraryAssetID     int64
	Provider           string
	SourceID           string
	CanonicalSourceURL string
	Title              string
	Author             string
}

func (LibraryMaterializationFacts) IngestionOrigin() Origin {
	return OriginLibraryMaterialization
}

type Request struct {
	OwnerUserID    int64
	StorageProfile string
	StorageKey     string
	URL            string
	SourceURL      string
	Origin         OriginFacts
}

type Handle struct {
	AssetID        int64
	PublicAssetID  string
	ExistingReady  bool
	OwnerUserID    int64
	StorageProfile string
	StorageKey     string
	Origin         OriginFacts
}

type Completion struct {
	StorageKey     string
	URL            string
	SourceURL      string
	MIMEType       string
	Width          int
	Height         int
	SizeBytes      int64
	ChecksumSHA256 string
	Animated       bool
}
