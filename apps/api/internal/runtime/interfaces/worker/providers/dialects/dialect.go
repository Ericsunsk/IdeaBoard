package dialects

import (
	"ideaboard/api/internal/workflow"
)

// Request is the unified image-execution input for every dialect. Media
// arrives as resolved public URLs in legacy order (input images, then mask,
// then canvas ref); dialects index it exactly as documented per shape.
// Transport facts (task id, feature metadata, cache policy, timeout) stay
// in the worker and never enter shaping.
type Request struct {
	Prompt    string
	Model     string
	Operation workflow.RuntimeOperation
	Images    []string
}

// Dialect shapes one vendor payload family. It is stateless and
// network-free: routing lives in Resolve (the single resolve-then-shape
// seam shared by execution and preview), egress lives in the gateway
// client, custom-params merge happens in the worker after Encode.
// CustomParamsJSON never enters here: the chat judgment already consumed it
// during dispatch. Response parsing stays unified in the package-level
// Decode (do not reimplement per dialect).
type Dialect interface {
	// Encode renders the vendor JSON body for req.
	Encode(Request) ([]byte, error)
}
