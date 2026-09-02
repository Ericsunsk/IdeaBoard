package tasks

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTaskInputManifestRoundTripPreservesImmutableObjectFacts(t *testing.T) {
	input := validManifestRef("asset_a", "derivative_a")
	original := validManifestRef("asset_original", "derivative_original")
	manifest, err := NewTaskInputManifest([]TaskInputManifestRef{input}, nil, &original, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := manifest.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "url") {
		t.Fatalf("manifest must not persist any URL: %s", raw)
	}
	decoded, err := UnmarshalTaskInputManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Inputs[0].ObjectGeneration != input.ObjectGeneration || decoded.Inputs[0].ChecksumSHA256 != input.ChecksumSHA256 {
		t.Fatalf("immutable object facts changed: %+v", decoded.Inputs[0])
	}
	if decoded.Original == nil || decoded.Original.AssetID != original.AssetID {
		t.Fatalf("original media was not preserved: %+v", decoded.Original)
	}
}

func TestTaskInputManifestConstructorCopiesInputs(t *testing.T) {
	refs := []TaskInputManifestRef{validManifestRef("asset_a", "derivative_a")}
	manifest, err := NewTaskInputManifest(refs, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	refs[0].ObjectKey = "mutated"
	if manifest.Inputs[0].ObjectKey == "mutated" {
		t.Fatal("manifest aliases caller-owned input")
	}
}

func TestTaskInputManifestAllowsEmptyMedia(t *testing.T) {
	manifest, err := NewTaskInputManifest(nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.ResolveInputs()) != 0 {
		t.Fatalf("empty manifest resolved refs: %+v", manifest.ResolveInputs())
	}
}

func TestTaskInputManifestRejectsIncompleteIdentity(t *testing.T) {
	ref := validManifestRef("asset_a", "derivative_a")
	ref.StorageProfileID = ""
	if _, err := NewTaskInputManifest([]TaskInputManifestRef{ref}, nil, nil, nil, nil); err == nil {
		t.Fatal("expected incomplete object identity rejection")
	}
}

func TestTaskInputManifestRejectsInvalidIntegrityFacts(t *testing.T) {
	ref := validManifestRef("asset_a", "derivative_a")
	ref.ChecksumSHA256 = "bad"
	if _, err := NewTaskInputManifest([]TaskInputManifestRef{ref}, nil, nil, nil, nil); err == nil {
		t.Fatal("expected invalid checksum rejection")
	}
}

func TestTaskInputManifestRejectsUnknownSchema(t *testing.T) {
	manifest := &TaskInputManifest{SchemaVersion: TaskInputManifestSchemaVersion + 1}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UnmarshalTaskInputManifest(raw); err == nil {
		t.Fatal("expected schema mismatch rejection")
	}
}

func validManifestRef(assetID, derivativeID string) TaskInputManifestRef {
	return TaskInputManifestRef{
		AssetID:          assetID,
		DerivativeID:     derivativeID,
		DerivativeTier:   TierProcessingInput,
		StorageProfileID: "profile_primary",
		ObjectKey:        "uploads/processing/input.webp",
		ObjectGeneration: 3,
		ChecksumSHA256:   strings.Repeat("a", 64),
		MIMEType:         "image/webp",
		SizeBytes:        1024,
		Role:             MediaRoleInput,
	}
}
