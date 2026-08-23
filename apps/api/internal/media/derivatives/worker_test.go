package derivatives

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"ideaboard/api/internal/media/storage"
)

func TestScanDerivativeOrphanObjectsContinuesAfterProfileFailure(t *testing.T) {
	expectedKey, err := storage.ImageDerivativeObjectKey(
		"uploads/tasks/source.png",
		"asset-expected",
		"preview",
		"webp",
	)
	if err != nil {
		t.Fatal(err)
	}
	orphanKey, err := storage.ImageDerivativeObjectKey(
		"uploads/tasks/source.png",
		"asset-orphan",
		"preview",
		"webp",
	)
	if err != nil {
		t.Fatal(err)
	}

	profiles := []storage.StorageProfile{
		{ID: "profile-failed", Enabled: true},
		{ID: "profile-disabled", Enabled: false},
		{ID: "profile-ready", Enabled: true},
	}
	calledProfiles := make([]string, 0, 2)
	result := scanDerivativeOrphanObjects(
		context.Background(),
		profiles,
		map[string]map[string]struct{}{
			"profile-ready": {expectedKey: {}},
		},
		func(_ context.Context, profileID string, prefix string) ([]string, error) {
			calledProfiles = append(calledProfiles, profileID)
			if prefix != "uploads/" {
				t.Fatalf("prefix=%q, want uploads/", prefix)
			}
			if profileID == "profile-failed" {
				return nil, errors.New("access denied")
			}
			return []string{expectedKey, orphanKey, "uploads/tasks/source.png"}, nil
		},
	)

	if !reflect.DeepEqual(calledProfiles, []string{"profile-failed", "profile-ready"}) {
		t.Fatalf("called profiles=%v", calledProfiles)
	}
	if len(result.failures) != 1 || result.failures[0].storageProfileID != "profile-failed" {
		t.Fatalf("failures=%+v", result.failures)
	}
	if len(result.findings) != 1 {
		t.Fatalf("findings=%+v", result.findings)
	}
	if finding := result.findings[0]; finding.storageProfileID != "profile-ready" || finding.count != 1 {
		t.Fatalf("finding=%+v", finding)
	}
}

func TestScanDerivativeOrphanObjectsReportsNoFailureForExpectedObjects(t *testing.T) {
	expectedKey, err := storage.ImageDerivativeObjectKey(
		"uploads/tasks/source.png",
		"asset-expected",
		"preview",
		"webp",
	)
	if err != nil {
		t.Fatal(err)
	}

	result := scanDerivativeOrphanObjects(
		context.Background(),
		[]storage.StorageProfile{{ID: "profile-ready", Enabled: true}},
		map[string]map[string]struct{}{
			"profile-ready": {expectedKey: {}},
		},
		func(context.Context, string, string) ([]string, error) {
			return []string{expectedKey}, nil
		},
	)

	if len(result.failures) != 0 || len(result.findings) != 0 {
		t.Fatalf("result=%+v", result)
	}
}
