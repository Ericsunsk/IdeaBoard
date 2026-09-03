package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	TaskInputManifestSchemaVersion = 1
	TierProcessingInput            = "processing-input"
)

// TaskInputManifest is the immutable media snapshot persisted with a Task.
// It contains storage identity and integrity facts, never a public or signed
// URL. A Worker may therefore retry the exact same object generation and
// issue a fresh provider URL just-in-time.
type TaskInputManifest struct {
	SchemaVersion int                    `json:"schema_version"`
	Inputs        []TaskInputManifestRef `json:"inputs,omitempty"`
	References    []TaskInputManifestRef `json:"references,omitempty"`
	Original      *TaskInputManifestRef  `json:"original,omitempty"`
	Mask          *TaskInputManifestRef  `json:"mask,omitempty"`
	UseMask       *bool                  `json:"use_mask,omitempty"`
}

type TaskInputManifestRef struct {
	AssetID          string  `json:"asset_id"`
	DerivativeID     string  `json:"derivative_id"`
	DerivativeTier   string  `json:"derivative_tier"`
	StorageProfileID string  `json:"storage_profile_id"`
	ObjectKey        string  `json:"object_key"`
	ObjectGeneration int64   `json:"object_generation"`
	ChecksumSHA256   string  `json:"checksum_sha256"`
	MIMEType         string  `json:"mime_type"`
	SizeBytes        int64   `json:"size_bytes"`
	Role             string  `json:"role,omitempty"`
	Weight           float64 `json:"weight,omitempty"`
}

func NewTaskInputManifest(
	inputs []TaskInputManifestRef,
	references []TaskInputManifestRef,
	original *TaskInputManifestRef,
	mask *TaskInputManifestRef,
	useMask *bool,
) (*TaskInputManifest, error) {
	manifest := &TaskInputManifest{
		SchemaVersion: TaskInputManifestSchemaVersion,
		Inputs:        append([]TaskInputManifestRef(nil), inputs...),
		References:    append([]TaskInputManifestRef(nil), references...),
		Original:      cloneTaskInputManifestRef(original),
		Mask:          cloneTaskInputManifestRef(mask),
		UseMask:       cloneBool(useMask),
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return manifest, nil
}

func (m *TaskInputManifest) Validate() error {
	if m == nil {
		return fmt.Errorf("task input manifest is nil")
	}
	if m.SchemaVersion != TaskInputManifestSchemaVersion {
		return fmt.Errorf("task input manifest schema version mismatch: got %d, want %d", m.SchemaVersion, TaskInputManifestSchemaVersion)
	}
	if err := validateTaskInputManifestRefs("input", m.Inputs); err != nil {
		return err
	}
	if err := validateTaskInputManifestRefs("reference", m.References); err != nil {
		return err
	}
	if err := validateOptionalTaskInputManifestRef("original", m.Original); err != nil {
		return err
	}
	return validateOptionalTaskInputManifestRef("mask", m.Mask)
}

func validateTaskInputManifestRefs(kind string, refs []TaskInputManifestRef) error {
	for index, ref := range refs {
		if err := validateTaskInputManifestRef(kind, index, ref); err != nil {
			return err
		}
	}
	return nil
}

func validateOptionalTaskInputManifestRef(kind string, ref *TaskInputManifestRef) error {
	if ref == nil {
		return nil
	}
	return validateTaskInputManifestRef(kind, -1, *ref)
}

func validateTaskInputManifestRef(kind string, index int, ref TaskInputManifestRef) error {
	label := manifestRefLabel(kind, index)
	if !strings.HasPrefix(strings.TrimSpace(ref.AssetID), "asset_") {
		return fmt.Errorf("task input manifest %s asset id is invalid", label)
	}
	if !strings.HasPrefix(strings.TrimSpace(ref.DerivativeID), "derivative_") {
		return fmt.Errorf("task input manifest %s derivative id is invalid", label)
	}
	if ref.DerivativeTier != TierProcessingInput {
		return fmt.Errorf("task input manifest %s derivative tier %q is not allowed", label, ref.DerivativeTier)
	}
	if strings.TrimSpace(ref.StorageProfileID) == "" || strings.TrimSpace(ref.ObjectKey) == "" {
		return fmt.Errorf("task input manifest %s object identity is incomplete", label)
	}
	if ref.ObjectGeneration <= 0 {
		return fmt.Errorf("task input manifest %s object generation is invalid", label)
	}
	if !validTaskInputManifestIntegrity(ref) {
		return fmt.Errorf("task input manifest %s integrity facts are invalid", label)
	}
	return nil
}

func manifestRefLabel(kind string, index int) string {
	if index < 0 {
		return kind
	}
	return fmt.Sprintf("%s %d", kind, index)
}

func validTaskInputManifestIntegrity(ref TaskInputManifestRef) bool {
	return len(strings.TrimSpace(ref.ChecksumSHA256)) == 64 &&
		strings.HasPrefix(strings.ToLower(strings.TrimSpace(ref.MIMEType)), "image/") &&
		ref.SizeBytes > 0
}

func (m *TaskInputManifest) Marshal() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func (m *TaskInputManifest) ResolveInputs() []TaskInputManifestRef {
	if m == nil {
		return nil
	}
	refs := make([]TaskInputManifestRef, 0, len(m.Inputs)+len(m.References)+2)
	refs = append(refs, m.Inputs...)
	refs = append(refs, m.References...)
	if m.Original != nil {
		refs = append(refs, *m.Original)
	}
	if m.Mask != nil {
		refs = append(refs, *m.Mask)
	}
	return refs
}

func cloneTaskInputManifestRef(ref *TaskInputManifestRef) *TaskInputManifestRef {
	if ref == nil {
		return nil
	}
	copyRef := *ref
	return &copyRef
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}
