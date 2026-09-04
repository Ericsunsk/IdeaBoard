package tests

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"ideaboard/api/internal/media/storage/domain"
)

func TestScanDerivativeOrphanObjectsContinuesAfterProfileFailure(t *testing.T) {
	expectedKey, err := domain.ImageDerivativeObjectKey(
		"uploads/tasks/source.png",
		"asset-expected",
		"preview",
		"webp",
	)
	if err != nil {
		t.Fatal(err)
	}
	orphanKey, err := domain.ImageDerivativeObjectKey(
		"uploads/tasks/source.png",
		"asset-orphan",
		"preview",
		"webp",
	)
	if err != nil {
		t.Fatal(err)
	}

	profiles := []domain.StorageProfile{
		{ID: "profile-failed", Enabled: true},
		{ID: "profile-disabled", Enabled: false},
		{ID: "profile-ready", Enabled: true},
	}
	calledProfiles := make([]string, 0, 2)
	result := ScanDerivativeOrphanObjects(
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
	if len(result.Failures) != 1 || result.Failures[0].StorageProfileID != "profile-failed" {
		t.Fatalf("failures=%+v", result.Failures)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("findings=%+v", result.Findings)
	}
	if finding := result.Findings[0]; finding.StorageProfileID != "profile-ready" || finding.Count != 1 {
		t.Fatalf("finding=%+v", finding)
	}
}

func TestScanDerivativeOrphanObjectsReportsNoFailureForExpectedObjects(t *testing.T) {
	expectedKey, err := domain.ImageDerivativeObjectKey(
		"uploads/tasks/source.png",
		"asset-expected",
		"preview",
		"webp",
	)
	if err != nil {
		t.Fatal(err)
	}

	result := ScanDerivativeOrphanObjects(
		context.Background(),
		[]domain.StorageProfile{{ID: "profile-ready", Enabled: true}},
		map[string]map[string]struct{}{
			"profile-ready": {expectedKey: {}},
		},
		func(context.Context, string, string) ([]string, error) {
			return []string{expectedKey}, nil
		},
	)

	if len(result.Failures) != 0 || len(result.Findings) != 0 {
		t.Fatalf("result=%+v", result)
	}
}
