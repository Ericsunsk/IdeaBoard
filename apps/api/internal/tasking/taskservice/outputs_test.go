package taskservice

import (
	"strings"
	"testing"

	"ideaboard/api/internal/tasking/tasks"
)

func TestTaskResultWithoutReclaimedImageURLsKeepsFactsAndMarksAvailability(t *testing.T) {
	got := taskResultWithoutReclaimedImageURLs(`{"status":"success","image_url":"/uploads/tasks/1/out.png","image_urls":["/uploads/tasks/1/out.png"],"model":"gpt-image"}`)
	if strings.Contains(got, "/uploads/tasks/1/out.png") {
		t.Fatalf("retired URL leaked from read projection: %s", got)
	}
	if !strings.Contains(got, `"image_availability":"deleted_by_user"`) || !strings.Contains(got, `"model":"gpt-image"`) {
		t.Fatalf("read projection lost task facts: %s", got)
	}
}

func TestHasReclaimedAssetOutput(t *testing.T) {
	assetID := int64(12)
	if !hasReclaimedAssetOutput([]tasks.TaskOutput{{OutputKind: "asset", MediaAssetID: &assetID}}) {
		t.Fatal("empty asset URL should be treated as reclaimed")
	}
	if hasReclaimedAssetOutput([]tasks.TaskOutput{{OutputKind: "remote", MediaAssetID: &assetID}}) {
		t.Fatal("remote output should not be treated as reclaimed")
	}
}
