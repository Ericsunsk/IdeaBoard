package catalog

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/testdb"
)

// Repository must satisfy the routing PoolStore port: the sibling
// RoutingResolver reads entries and version through ListPool.
var _ routing.PoolStore = (*Repository)(nil)

func TestPoolInputErrorsPreserveRoutingCause(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{name: "service", run: func() error {
			_, err := normalizeService("")
			return err
		}},
		{name: "resolution", run: func() error {
			_, err := normalizeEntries([]PoolEntryInput{{ModelID: "openai/a", Resolutions: []string{"invalid"}}})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			var cause *routing.InvalidRoutingInputError
			if !errors.Is(err, ErrInvalidPoolInput) || !errors.As(err, &cause) {
				t.Fatalf("error = %v; want pool classification and routing cause", err)
			}
		})
	}
}

func newCatalogTestRepo(t *testing.T) *Repository {
	t.Helper()
	return NewRepository(testdb.Open(t))
}

func mustReplace(t *testing.T, r *Repository, service string, expected uint64, inputs []PoolEntryInput) ReplacePoolResult {
	t.Helper()
	result, err := r.ReplacePool(context.Background(), service, expected, inputs)
	if err != nil {
		t.Fatalf("ReplacePool(%q) failed: %v", service, err)
	}
	return result
}

func TestReplacePoolCreateStartsAtVersionOne(t *testing.T) {
	r := newCatalogTestRepo(t)
	ctx := context.Background()

	result := mustReplace(t, r, "generation", 0, []PoolEntryInput{
		{ModelID: "b/second", Enabled: true, Resolutions: []string{"2K", "4k"}},
		{ModelID: "a/first", Enabled: true, Resolutions: []string{"1k"}},
	})
	if result.Version != 1 || !result.Changed {
		t.Fatalf("result = %+v, want {Version:1 Changed:true}", result)
	}

	version, err := r.GetVersion(ctx, "generation")
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
	if version != 1 {
		t.Fatalf("version = %d, want 1", version)
	}

	pool, err := r.ListPool(ctx, "generation")
	if err != nil {
		t.Fatalf("ListPool failed: %v", err)
	}
	if pool.Service != "generation" || pool.Version != 1 {
		t.Fatalf("pool identity = %+v, want service generation version 1", pool)
	}
	if len(pool.Entries) != 2 || pool.Entries[0].Model != "a/first" || pool.Entries[1].Model != "b/second" {
		t.Fatalf("entries = %+v, want sorted a/first, b/second", pool.Entries)
	}
	if len(pool.Entries[1].SupportedResolutions) != 2 ||
		pool.Entries[1].SupportedResolutions[0] != routing.Resolution2K ||
		pool.Entries[1].SupportedResolutions[1] != routing.Resolution4K {
		t.Fatalf("b/second resolutions = %+v, want [2k 4k] normalized", pool.Entries[1].SupportedResolutions)
	}

}

func TestReplacePoolNoOpSaveDoesNotBump(t *testing.T) {
	r := newCatalogTestRepo(t)
	ctx := context.Background()
	inputs := []PoolEntryInput{
		{ModelID: "a/first", Enabled: true, Resolutions: []string{"1k", "2k"}},
	}
	mustReplace(t, r, "render", 0, inputs)

	// Same effective content in different order/case must not bump.
	result := mustReplace(t, r, "render", 1, []PoolEntryInput{
		{ModelID: "a/first", Enabled: true, Resolutions: []string{"2K", "1K"}},
	})
	if result.Version != 1 || result.Changed {
		t.Fatalf("no-op result = %+v, want {Version:1 Changed:false}", result)
	}
	if version, _ := r.GetVersion(ctx, "render"); version != 1 {
		t.Fatalf("version after no-op = %d, want 1", version)
	}
}

func TestReplacePoolVersionConflictWritesNothing(t *testing.T) {
	r := newCatalogTestRepo(t)
	ctx := context.Background()
	mustReplace(t, r, "inpaint", 0, []PoolEntryInput{
		{ModelID: "a/first", Enabled: true, Resolutions: []string{"1k"}},
	})

	_, err := r.ReplacePool(ctx, "inpaint", 0, []PoolEntryInput{
		{ModelID: "b/second", Enabled: true, Resolutions: []string{"1k"}},
	})
	var conflict *PoolVersionConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want *PoolVersionConflict", err)
	}
	if conflict.Expected != 0 || conflict.Current != 1 || conflict.Service != "inpaint" {
		t.Fatalf("conflict = %+v, want expected 0 current 1", conflict)
	}

	pool, err := r.ListPool(ctx, "inpaint")
	if err != nil {
		t.Fatalf("ListPool failed: %v", err)
	}
	if pool.Version != 1 || len(pool.Entries) != 1 || pool.Entries[0].Model != "a/first" {
		t.Fatalf("conflicted write leaked: %+v", pool)
	}
}

func TestReplacePoolEffectiveChangeBumps(t *testing.T) {
	r := newCatalogTestRepo(t)
	mustReplace(t, r, "variations", 0, []PoolEntryInput{
		{ModelID: "a/first", Enabled: true, Resolutions: []string{"1k"}},
		{ModelID: "b/second", Enabled: true, Resolutions: []string{"1k"}},
	})

	// Disabling one entry is an effective change: version 1 -> 2.
	result := mustReplace(t, r, "variations", 1, []PoolEntryInput{
		{ModelID: "a/first", Enabled: true, Resolutions: []string{"1k"}},
		{ModelID: "b/second", Enabled: false, Resolutions: []string{"1k"}},
	})
	if result.Version != 2 || !result.Changed {
		t.Fatalf("result = %+v, want {Version:2 Changed:true}", result)
	}
}

func TestReplacePoolClearAndEmptyRead(t *testing.T) {
	r := newCatalogTestRepo(t)

	// Clearing a never-written pool is a no-op at version 0.
	cleared := mustReplace(t, r, "cutout", 0, nil)
	if cleared.Version != 0 || cleared.Changed {
		t.Fatalf("clear-absent = %+v, want {Version:0 Changed:false}", cleared)
	}

	mustReplace(t, r, "cutout", 0, []PoolEntryInput{
		{ModelID: "a/first", Enabled: true, Resolutions: []string{"1k"}},
	})
	// Clearing a populated pool bumps: the service reads unavailable.
	cleared = mustReplace(t, r, "cutout", 1, nil)
	if cleared.Version != 2 || !cleared.Changed {
		t.Fatalf("clear = %+v, want {Version:2 Changed:true}", cleared)
	}
}

func TestReplacePoolValidation(t *testing.T) {
	r := newCatalogTestRepo(t)
	ctx := context.Background()
	cases := []struct {
		name    string
		service string
		inputs  []PoolEntryInput
	}{
		{"unknown service", "not-a-service", []PoolEntryInput{{ModelID: "a/x", Enabled: true, Resolutions: []string{"1k"}}}},
		{"vision is rejected", "vision", []PoolEntryInput{{ModelID: "a/x", Enabled: true, Resolutions: []string{"1k"}}}},
		{"blank service", "  ", []PoolEntryInput{{ModelID: "a/x", Enabled: true, Resolutions: []string{"1k"}}}},
		{"blank model", "fusion", []PoolEntryInput{{ModelID: "  ", Enabled: true, Resolutions: []string{"1k"}}}},
		{"default capability", "fusion", []PoolEntryInput{{ModelID: "a/x", Enabled: true, Resolutions: []string{"default"}}}},
		{"unknown resolution", "fusion", []PoolEntryInput{{ModelID: "a/x", Enabled: true, Resolutions: []string{"8k"}}}},
		{"enabled without resolutions", "fusion", []PoolEntryInput{{ModelID: "a/x", Enabled: true}}},
		{"exact duplicate model", "fusion", []PoolEntryInput{
			{ModelID: "a/x", Enabled: true, Resolutions: []string{"1k"}},
			{ModelID: "a/x", Enabled: true, Resolutions: []string{"2k"}},
		}},
		{"case-insensitive duplicate model", "fusion", []PoolEntryInput{
			{ModelID: "A/x", Enabled: true, Resolutions: []string{"1k"}},
			{ModelID: "a/X", Enabled: true, Resolutions: []string{"2k"}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := r.ReplacePool(ctx, tc.service, 0, tc.inputs); !errors.Is(err, ErrInvalidPoolInput) {
				t.Fatalf("err = %v, want ErrInvalidPoolInput", err)
			}
		})
	}
	if version, _ := r.GetVersion(ctx, "fusion"); version != 0 {
		t.Fatalf("failed writes must not create versions, got %d", version)
	}
}

func TestReplacePoolDisabledEntryMayOmitResolutions(t *testing.T) {
	r := newCatalogTestRepo(t)
	result := mustReplace(t, r, "upscale", 0, []PoolEntryInput{
		{ModelID: "a/first", Enabled: false},
	})
	if result.Version != 1 || !result.Changed {
		t.Fatalf("result = %+v, want {Version:1 Changed:true}", result)
	}
}

func TestListPoolAbsentServiceReadsEmpty(t *testing.T) {
	r := newCatalogTestRepo(t)
	ctx := context.Background()
	pool, err := r.ListPool(ctx, "generation")
	if err != nil {
		t.Fatalf("ListPool failed: %v", err)
	}
	if pool.Service != "generation" || pool.Version != 0 || len(pool.Entries) != 0 {
		t.Fatalf("pool = %+v, want empty pool at version 0", pool)
	}
	if version, err := r.GetVersion(ctx, "generation"); err != nil || version != 0 {
		t.Fatalf("version = %d, err = %v, want 0, nil", version, err)
	}
}

func TestPoolVersionsArePerService(t *testing.T) {
	r := newCatalogTestRepo(t)
	ctx := context.Background()
	mustReplace(t, r, "generation", 0, []PoolEntryInput{
		{ModelID: "a/first", Enabled: true, Resolutions: []string{"1k"}},
	})
	mustReplace(t, r, "generation", 1, []PoolEntryInput{
		{ModelID: "a/first", Enabled: true, Resolutions: []string{"1k", "2k"}},
	})
	mustReplace(t, r, "render", 0, []PoolEntryInput{
		{ModelID: "a/first", Enabled: true, Resolutions: []string{"1k"}},
	})
	genVersion, _ := r.GetVersion(ctx, "generation")
	renderVersion, _ := r.GetVersion(ctx, "render")
	if genVersion != 2 || renderVersion != 1 {
		t.Fatalf("versions = generation %d render %d, want 2 and 1", genVersion, renderVersion)
	}
}
