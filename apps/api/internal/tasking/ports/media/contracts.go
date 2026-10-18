package media

import (
	"context"

	tasks "ideaboard/api/internal/tasking/domain"
)

type UploadReader interface {
	ReadUploadObject(ctx context.Context, ref string, maxBytes int64) (*StoredObject, error)
}

// RuntimeMediaResolver turns durable Asset facts into provider-safe runtime
// refs without mutating the persisted Task Params shape.
type RuntimeMediaResolver interface {
	ResolveRuntimeMedia(context.Context, *tasks.WorkflowTaskParams) (*tasks.RuntimeMediaInputs, error)
}

// ProcessingInputReadiness validates that every durable image Asset used by a
// task already has the provider-neutral processing derivative materialized.
// Submission and runtime resolution share this seam so a task is never
// charged and queued only to fail later because its input derivative is still
// being rendered.
type ProcessingInputReadiness interface {
	ValidateProcessingInputReadiness(context.Context, int64, []int64) error
}

// TaskInputManifestBuilder resolves mutable Media Asset references into the
// immutable object facts persisted with a Task. Implementations must verify
// every referenced object before returning.
type TaskInputManifestBuilder interface {
	BuildTaskInputManifest(context.Context, int64, *tasks.MediaInputs) (*tasks.TaskInputManifest, error)
}

type ResultMaterializer interface {
	MaterializeRemoteResults(ctx context.Context, userID int64, taskID string, sourceURLs []string, opts ResultMaterializeOptions) (MaterializedResults, error)
	MaterializeBytesResult(ctx context.Context, userID int64, taskID string, filename string, sourceURL string, data []byte, opts ResultMaterializeOptions) (MaterializedResults, error)
}

// GenerationMediaPreparer materializes external or temporary inputs before
// Task submission. It must not use the Task/Billing/River transaction.
type GenerationMediaPreparer interface {
	PrepareGenerationMedia(context.Context, int64, string, *tasks.GenerationRequestSnapshot) (tasks.GenerationRequestSnapshot, error)
}

type ResultMaterializeOptions struct {
	AllowHosts []string
}

type MaterializedResults struct {
	Items []MaterializedResult
}

func (r MaterializedResults) ImageURLs() []string {
	urls := make([]string, 0, len(r.Items))
	for _, item := range r.Items {
		urls = append(urls, item.ImageURL)
	}
	return urls
}

func (r MaterializedResults) Outputs() []tasks.TaskOutput {
	outputs := make([]tasks.TaskOutput, 0, len(r.Items))
	for _, item := range r.Items {
		outputs = append(outputs, item.Output)
	}
	return outputs
}

type MaterializedResult struct {
	ImageURL string
	Output   tasks.TaskOutput
}

type StoredObject struct {
	ObjectKey   string
	Filename    string
	ContentType string
	Data        []byte
	MIMEType    string
	Ext         string
}
