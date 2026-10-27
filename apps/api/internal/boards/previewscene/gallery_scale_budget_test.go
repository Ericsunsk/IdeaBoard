package previewscene

import "testing"

// Gallery scale budgets for Issue #928: large Boards and multi-card
// Galleries must keep bounded cost. Per-card scenes are capped by
// CatalogLimits (see contract.md §Budgets); the list payload then grows
// at most linearly in card count, never in document size.

func TestGalleryScalePerCardBudgetConverges(t *testing.T) {
	for _, nodes := range []int{10, 100, 2000} {
		envelope := largeCatalogEnvelope(t, nodes)
		_, sceneJSON, ok := DeriveCatalog(envelope)
		if !ok {
			t.Fatalf("nodes=%d: large board must derive a catalog scene", nodes)
		}
		if int64(len(sceneJSON)) > CatalogMaxSceneBytes {
			t.Fatalf("nodes=%d: scene = %d bytes, want <= %d", nodes, len(sceneJSON), CatalogMaxSceneBytes)
		}
		scene, err := ParseScene(sceneJSON)
		if err != nil {
			t.Fatalf("nodes=%d: budgeted scene does not parse: %v", nodes, err)
		}
		if scene.Budgets.MaxSceneBytes == nil || *scene.Budgets.MaxSceneBytes != CatalogMaxSceneBytes {
			t.Fatalf("nodes=%d: scene must self-describe its byte budget: %+v", nodes, scene.Budgets)
		}
		if int64(len(scene.Nodes)) > CatalogMaxNodes {
			t.Fatalf("nodes=%d: scene nodes = %d, want <= %d", nodes, len(scene.Nodes), CatalogMaxNodes)
		}
		if int64(len(scene.Connections)) > CatalogMaxConnections {
			t.Fatalf("nodes=%d: scene connections = %d, want <= %d", nodes, len(scene.Connections), CatalogMaxConnections)
		}
		media := 0
		for _, n := range scene.Nodes {
			media += len(n.Media)
		}
		if int64(media) > CatalogMaxMediaRefs {
			t.Fatalf("nodes=%d: scene media refs = %d, want <= %d", nodes, media, CatalogMaxMediaRefs)
		}
	}
}

func TestGalleryScaleLargeBoardsConvergeToSameBound(t *testing.T) {
	_, small, ok := DeriveCatalog(largeCatalogEnvelope(t, 2000))
	if !ok {
		t.Fatal("2000-node board must derive a catalog scene")
	}
	_, large, ok := DeriveCatalog(largeCatalogEnvelope(t, 5000))
	if !ok {
		t.Fatal("5000-node board must derive a catalog scene")
	}
	if int64(len(small)) > CatalogMaxSceneBytes || int64(len(large)) > CatalogMaxSceneBytes {
		t.Fatalf("large scenes must converge: 2000=%d 5000=%d want <= %d",
			len(small), len(large), CatalogMaxSceneBytes)
	}
	// Same-shape boards at different counts share the kept prefix and the
	// per-card caps; only full-layout bounds, content identity, and
	// omission counters vary, so sizes stay within a small delta.
	if diff := len(small) - len(large); diff < -1024 || diff > 1024 {
		t.Fatalf("same-shape large boards must converge: 2000=%d 5000=%d want delta <= 1KB",
			len(small), len(large))
	}
}

// Truncation sheds trailing prefixes only: the full-layout bounds computed
// over every node frame before budgeting must survive unchanged, so the
// Gallery keeps the whole-board spatial overview.
func TestGalleryScaleTruncationKeepsFullLayoutBounds(t *testing.T) {
	stored := largeCatalogEnvelope(t, 2000)
	snapshot, ok := snapshotForExtraction(stored)
	if !ok {
		t.Fatal("large envelope must be extractable")
	}
	full, err := Extract(snapshot, "cv-full-bounds")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	_, cappedJSON, ok := DeriveCatalog(stored)
	if !ok {
		t.Fatal("large board must derive a catalog scene")
	}
	capped, err := ParseScene(cappedJSON)
	if err != nil {
		t.Fatalf("ParseScene(capped): %v", err)
	}
	if !capped.Truncated {
		t.Fatal("2000-node board must report truncation")
	}
	if full.Bounds == nil || capped.Bounds == nil {
		t.Fatal("both full and capped scenes must carry bounds")
	}
	if *full.Bounds != *capped.Bounds {
		t.Fatalf("truncation must keep full-layout bounds: full=%+v capped=%+v", full.Bounds, capped.Bounds)
	}
	if full.Extent != capped.Extent {
		t.Fatalf("truncation must keep the normalized extent: full=%+v capped=%+v", full.Extent, capped.Extent)
	}
}
