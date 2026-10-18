package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func validAssetInput() AssetInput {
	return AssetInput{
		Provider:    ProviderUnsplash,
		SourceID:    "photo-123",
		SourceURL:   "https://unsplash.com/photos/photo-123",
		PreviewURL:  "https://images.unsplash.com/photo-123?w=800",
		OriginalURL: "https://images.unsplash.com/photo-123",
		Title:       "A photo",
		Author:      "Author",
		Width:       1200,
		Height:      800,
	}
}

func TestNormalizeAndValidateAssetNormalizesInput(t *testing.T) {
	input := validAssetInput()
	input.Provider = " UNSPLASH "
	input.SourceID = " photo-123 "
	input.SourceURL = " https://unsplash.com/photos/photo-123 "
	got, err := NormalizeAndValidateAsset(input)
	if err != nil {
		t.Fatalf("NormalizeAndValidateAsset returned error: %v", err)
	}
	if got.Provider != ProviderUnsplash || got.SourceID != "photo-123" || got.SourceURL != "https://unsplash.com/photos/photo-123" {
		t.Fatalf("normalized asset = %#v", got)
	}
}

func TestNormalizeAndValidateAssetRejectsUntrustedURLs(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{name: "http", url: "http://images.unsplash.com/photo-123"},
		{name: "userinfo", url: "https://user:pass@images.unsplash.com/photo-123"},
		{name: "lookalike", url: "https://images.unsplash.com.attacker.example/photo-123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := validAssetInput()
			input.PreviewURL = tc.url
			if _, err := NormalizeAndValidateAsset(input); err != ErrInvalidSource {
				t.Fatalf("error = %v, want %v", err, ErrInvalidSource)
			}
		})
	}
}

func TestNormalizeAndValidateAssetRejectsMissingURLsWithoutFallback(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*AssetInput)
	}{
		{"missing source_url", func(input *AssetInput) { input.SourceURL = "" }},
		{"missing original_url", func(input *AssetInput) { input.OriginalURL = "" }},
		{"missing preview_url", func(input *AssetInput) { input.PreviewURL = "" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := validAssetInput()
			tt.mutate(&input)
			if _, err := NormalizeAndValidateAsset(input); err != ErrInvalidSource {
				t.Fatalf("error = %v, want %v (no preview fallback fill)", err, ErrInvalidSource)
			}
		})
	}
}

func TestNormalizeAndValidateAssetRejectsUnsupportedProvider(t *testing.T) {
	input := validAssetInput()
	input.Provider = "google"
	if _, err := NormalizeAndValidateAsset(input); err != ErrInvalidProvider {
		t.Fatalf("error = %v, want %v", err, ErrInvalidProvider)
	}
}

func TestNormalizeFolderName(t *testing.T) {
	if got, err := NormalizeFolderName("  Travel  "); err != nil || got != "Travel" {
		t.Fatalf("NormalizeFolderName = %q, %v", got, err)
	}
	for _, raw := range []string{"", "   "} {
		if _, err := NormalizeFolderName(raw); err != ErrInvalidFolderName {
			t.Fatalf("NormalizeFolderName(%q) error = %v, want %v", raw, err, ErrInvalidFolderName)
		}
	}
}

func TestParseFolderSelection(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want FolderSelection
	}{
		{name: "default all", raw: "", want: FolderSelection{Kind: FolderSelectionAll}},
		{name: "all", raw: "all", want: FolderSelection{Kind: FolderSelectionAll}},
		{name: "quick saves", raw: "quick-saves", want: FolderSelection{Kind: FolderSelectionQuickSaves}},
		{name: "user folder", raw: "42", want: FolderSelection{Kind: FolderSelectionUser, ID: 42}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseFolderSelection(tc.raw)
			if err != nil {
				t.Fatalf("ParseFolderSelection returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("selection = %#v, want %#v", got, tc.want)
			}
		})
	}
	for _, raw := range []string{"0", "-1", "wat"} {
		if _, err := ParseFolderSelection(raw); err != ErrFolderNotFound {
			t.Fatalf("ParseFolderSelection(%q) error = %v, want %v", raw, err, ErrFolderNotFound)
		}
	}
}

func validManagedAssetInput() V1SaveManagedAssetInput {
	return V1SaveManagedAssetInput{
		MediaAssetID: "asset_0123456789abcdef01234567",
		Title:        "hero",
		Author:       "maker",
	}
}

func TestNormalizeAndValidateManagedAssetAcceptsManualAndTaskShapes(t *testing.T) {
	manual, err := NormalizeAndValidateManagedAsset(validManagedAssetInput())
	if err != nil || manual.Lineage != nil {
		t.Fatalf("manual = %+v, %v; want accepted without lineage", manual, err)
	}
	withLineage := validManagedAssetInput()
	withLineage.Lineage = &TaskLineage{TaskID: "task_aaaaaaaaaaaaaaaaaaaaaaaa", ResultIndex: 3}
	got, err := NormalizeAndValidateManagedAsset(withLineage)
	if err != nil || got.Lineage == nil || got.Lineage.TaskID != "task_aaaaaaaaaaaaaaaaaaaaaaaa" || got.Lineage.ResultIndex != 3 {
		t.Fatalf("task = %+v, %v; want accepted provenance", got, err)
	}
}

func TestNormalizeAndValidateManagedAssetRejects(t *testing.T) {
	valid := validManagedAssetInput()
	for _, tt := range []struct {
		name   string
		mutate func(*V1SaveManagedAssetInput)
	}{
		{"bad media id", func(m *V1SaveManagedAssetInput) { m.MediaAssetID = "nope" }},
		{"empty media id", func(m *V1SaveManagedAssetInput) { m.MediaAssetID = "  " }},
		{"long title", func(m *V1SaveManagedAssetInput) { m.Title = string(make([]byte, 300)) }},
		{"bad task id", func(m *V1SaveManagedAssetInput) { m.Lineage = &TaskLineage{TaskID: "nope", ResultIndex: 0} }},
		{"negative index", func(m *V1SaveManagedAssetInput) {
			m.Lineage = &TaskLineage{TaskID: "task_aaaaaaaaaaaaaaaaaaaaaaaa", ResultIndex: -1}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := valid
			tt.mutate(&input)
			if _, err := NormalizeAndValidateManagedAsset(input); err != ErrInvalidSource {
				t.Fatalf("error = %v, want %v", err, ErrInvalidSource)
			}
		})
	}
}

func TestPairDimensionsKeepsPairs(t *testing.T) {
	w, h := int32(100), int32(80)
	gotW, gotH := PairDimensions(&w, &h)
	if gotW == nil || *gotW != 100 || gotH == nil || *gotH != 80 {
		t.Fatalf("pair = %v x %v, want 100 x 80", gotW, gotH)
	}
	zero := int32(0)
	for _, tt := range []struct {
		name  string
		width *int32
		vert  *int32
	}{
		{"both nil", nil, nil},
		{"half known", &w, nil},
		{"zero", &zero, &h},
	} {
		if gotW, gotH := PairDimensions(tt.width, tt.vert); gotW != nil || gotH != nil {
			t.Fatalf("%s = %v x %v, want nil pair", tt.name, gotW, gotH)
		}
	}
}

// merged from fixtures_test.go (domain aggregation: library source fixtures contract (types.go) and materialization status guards (v1.go)).
// TestLibrarySourceFixturesContract pins the unified Library Source wire
// rules (issues #930/#931) against the canonical fixtures: external rows
// carry provider/source identity plus real URLs and a complete positive
// dimension pair; media rows carry a media_asset_id plus a paired
// (both-null-or-both-positive) dimension and MUST NOT contain
// provider/source_id/source_url/preview_url/original_url keys. Task
// lineage is provenance only; pagination keeps next_cursor/has_more.
func TestLibrarySourceFixturesContract(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "contracts", "library", "library-source.fixtures.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var fixtures map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatalf("parse fixtures: %v", err)
	}
	decode := func(t *testing.T, key string) map[string]any {
		t.Helper()
		raw, ok := fixtures[key]
		if !ok {
			t.Fatalf("fixture %q missing", key)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parse %q: %v", key, err)
		}
		return doc
	}

	external := decode(t, "external")
	assertExternalShape(t, external)
	for _, key := range []string{"media_known", "media_unknown", "task_lineage"} {
		assertMediaShape(t, decode(t, key))
	}

	mixed := decode(t, "mixed_list")
	items, ok := mixed["data"].([]any)
	if !ok || len(items) != 3 {
		t.Fatalf("mixed_list has %d items, want 3", len(items))
	}
	seen := map[string]bool{}
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			t.Fatal("mixed_list item is not an object")
		}
		source, _ := row["source"].(string)
		seen[source] = true
		switch source {
		case "external":
			assertExternalShape(t, row)
		case "media":
			assertMediaShape(t, row)
		default:
			t.Fatalf("mixed_list source = %q, want external|media", source)
		}
		if row["display"] == nil {
			t.Fatalf("mixed_list %v lacks inline display", row["id"])
		}
	}
	if !seen["external"] || !seen["media"] {
		t.Fatalf("mixed_list sources = %v, want both", seen)
	}
	meta, ok := mixed["meta"].(map[string]any)
	if !ok {
		t.Fatal("mixed_list lacks meta")
	}
	if _, ok := meta["next_cursor"]; !ok {
		t.Fatal("mixed_list meta lacks next_cursor")
	}
	if _, ok := meta["has_more"]; !ok {
		t.Fatal("mixed_list meta lacks has_more")
	}

	lineage := decode(t, "task_lineage")
	if lineage["task_id"] == nil || lineage["task_result_index"] == nil {
		t.Fatal("task_lineage lacks provenance pair")
	}
	for _, key := range []string{"provider", "source_id", "source_url", "preview_url", "original_url", "saved_by_user_id"} {
		if _, present := lineage[key]; present {
			t.Fatalf("task_lineage carries %q", key)
		}
	}
}

func assertExternalShape(t *testing.T, row map[string]any) {
	t.Helper()
	if row["source"] != "external" {
		t.Fatalf("source = %v, want external", row["source"])
	}
	for _, key := range []string{"provider", "source_id", "source_url", "preview_url", "original_url"} {
		value, _ := row[key].(string)
		if value == "" {
			t.Fatalf("external %q is empty", key)
		}
	}
	if _, present := row["media_asset_id"]; present {
		t.Fatal("external row carries media_asset_id")
	}
	width, wok := row["width"].(float64)
	height, hok := row["height"].(float64)
	if !wok || !hok || width <= 0 || height <= 0 {
		t.Fatalf("external dims = %v x %v, want complete positive pair", row["width"], row["height"])
	}
}

func assertMediaShape(t *testing.T, row map[string]any) {
	t.Helper()
	if row["source"] != "media" {
		t.Fatalf("source = %v, want media", row["source"])
	}
	for _, key := range []string{"provider", "source_id", "source_url", "preview_url", "original_url", "saved_by_user_id"} {
		if _, present := row[key]; present {
			t.Fatalf("media row carries %q", key)
		}
	}
	link, _ := row["media_asset_id"].(string)
	if link == "" {
		t.Fatal("media row lacks media_asset_id")
	}
	width, hasWidth := row["width"]
	height, hasHeight := row["height"]
	if !hasWidth || !hasHeight {
		t.Fatal("media row lacks width/height keys (null = unknown)")
	}
	widthNum, wok := width.(float64)
	heightNum, hok := height.(float64)
	switch {
	case width == nil && height == nil:
	case wok && hok && widthNum > 0 && heightNum > 0:
	default:
		t.Fatalf("media dims = %v x %v, want paired null-or-positive", width, height)
	}
}

// merged from status_test.go (domain aggregation: library source fixtures contract (types.go) and materialization status guards (v1.go)).
// TestCanTransitionMirrorsSQLGuards pins the durable materialization
// transition table to the guards in
// internal/db/queries/workspace_library.sql: StartLibraryMaterialization
// fences pending -> processing (public running); Complete/Fail flip a
// live (pending/processing) job to completed/failed, so direct
// pending -> completed|failed is allowed; terminals have no outgoing
// transitions. Every true entry was already possible before typing.
func TestCanTransitionMirrorsSQLGuards(t *testing.T) {
	allowed := [][2]V1MaterializationStatus{
		{V1MaterializationPending, V1MaterializationRunning},
		{V1MaterializationPending, V1MaterializationCompleted},
		{V1MaterializationPending, V1MaterializationFailed},
		{V1MaterializationRunning, V1MaterializationCompleted},
		{V1MaterializationRunning, V1MaterializationFailed},
	}
	for _, transition := range allowed {
		if !CanTransition(transition[0], transition[1]) {
			t.Errorf("CanTransition(%q, %q) = false, want true", transition[0], transition[1])
		}
	}
	terminals := []V1MaterializationStatus{V1MaterializationCompleted, V1MaterializationFailed}
	live := []V1MaterializationStatus{
		V1MaterializationPending, V1MaterializationRunning,
		V1MaterializationCompleted, V1MaterializationFailed,
	}
	for _, from := range terminals {
		for _, to := range live {
			if CanTransition(from, to) {
				t.Errorf("CanTransition(%q, %q) = true, want false (terminal has no outgoing transitions)", from, to)
			}
		}
	}
	if CanTransition(V1MaterializationPending, V1MaterializationPending) {
		t.Error("CanTransition(pending, pending) = true, want false (claim converges without a flip, never pending -> pending)")
	}
	if CanTransition(V1MaterializationRunning, V1MaterializationRunning) {
		t.Error("CanTransition(running, running) = true, want false (no self-transition guard exists)")
	}
	if CanTransition(V1MaterializationRunning, V1MaterializationPending) {
		t.Error("CanTransition(running, pending) = true, want false (failed jobs retry by claiming a new job, never a backward flip)")
	}
}

// TestMaterializationStatusWireWords pins the wire/DB words: typing must
// not change the strings on the wire or in the database. The durable
// word for running stays 'processing' (mapped at the store boundary).
func TestMaterializationStatusWireWords(t *testing.T) {
	for status, want := range map[V1MaterializationStatus]string{
		V1MaterializationPending:   "pending",
		V1MaterializationRunning:   "running",
		V1MaterializationCompleted: "completed",
		V1MaterializationFailed:    "failed",
	} {
		if string(status) != want {
			t.Errorf("V1MaterializationStatus = %q, want %q", string(status), want)
		}
	}
	for dbWord, want := range map[string]V1MaterializationStatus{
		"pending":    V1MaterializationPending,
		"processing": V1MaterializationRunning,
		"completed":  V1MaterializationCompleted,
		"failed":     V1MaterializationFailed,
	} {
		got, err := V1MaterializationStatusFromDB(dbWord)
		if err != nil || got != want {
			t.Errorf("V1MaterializationStatusFromDB(%q) = (%q, %v), want (%q, nil)", dbWord, got, err, want)
		}
	}
	if _, err := V1MaterializationStatusFromDB("exploding"); err == nil {
		t.Error("V1MaterializationStatusFromDB(exploding) = nil, want error (unknown statuses fail closed)")
	}
}

// TestResolveStatusWireWords pins the resolve outcome vocabulary: typing
// must not change the strings on the wire.
func TestResolveStatusWireWords(t *testing.T) {
	if string(V1ResolveResolved) != "resolved" {
		t.Errorf("V1ResolveResolved = %q, want %q", string(V1ResolveResolved), "resolved")
	}
	if string(V1ResolveUnresolved) != "unresolved" {
		t.Errorf("V1ResolveUnresolved = %q, want %q", string(V1ResolveUnresolved), "unresolved")
	}
}
