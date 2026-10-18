package worker

// Workspace provider-result materializer tests: inline data: results
// ingest without network, https: results ride the fetcher seam, and every
// misconfiguration fails closed. No secrets, no live network: all bytes
// are local fixtures.

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"ideaboard/api/internal/media/workspacemedia"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
	"ideaboard/api/internal/tasking/workspacetasks"
)

func materializerTestDeps(media *workspacemedia.InMemoryWorkspaceMediaStore, objects *fakeRendererObjects) taskruntime.WorkspaceProviderResultMaterializerDeps {
	return taskruntime.WorkspaceProviderResultMaterializerDeps{
		Media:   media,
		Objects: objects,
		Fetcher: taskruntime.WorkspaceResultBytesFetchFunc(func(_ context.Context, _ *workspacetasks.WorkspaceTask, _ workspacetasks.OutputRef) ([]byte, error) {
			return nil, errors.New("fake: fetcher must not run for inline results")
		}),
	}
}

func materializerTestScope(ctx context.Context) context.Context {
	return taskruntime.WithWorkspaceTaskScope(ctx, "ws_test")
}

func TestNewWorkspaceProviderResultMaterializerRequiresSeams(t *testing.T) {
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	full := materializerTestDeps(media, &fakeRendererObjects{})
	if _, err := taskruntime.NewWorkspaceProviderResultMaterializer(full); err != nil {
		t.Fatalf("full deps = %v, want nil", err)
	}
	noMedia := full
	noMedia.Media = nil
	if _, err := taskruntime.NewWorkspaceProviderResultMaterializer(noMedia); err == nil {
		t.Fatal("nil media = nil, want error")
	}
	noObjects := full
	noObjects.Objects = nil
	if _, err := taskruntime.NewWorkspaceProviderResultMaterializer(noObjects); err == nil {
		t.Fatal("nil objects = nil, want error")
	}
	noFetcher := full
	noFetcher.Fetcher = nil
	if _, err := taskruntime.NewWorkspaceProviderResultMaterializer(noFetcher); err == nil {
		t.Fatal("nil fetcher = nil, want error")
	}
}

func TestWorkspaceProviderMaterializerMissingScopeFailsClosed(t *testing.T) {
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	materializer, err := taskruntime.NewWorkspaceProviderResultMaterializer(materializerTestDeps(media, &fakeRendererObjects{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	png := fixturePNGBytes(t)
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	// No scope stamped: must fail closed, never ingest.
	if _, err := materializer.MaterializeRemoteResults(context.Background(), 7, "task_noscope", []string{dataURL}, taskmedia.ResultMaterializeOptions{}); err == nil {
		t.Fatal("missing scope = nil, want fail-closed")
	}
	if _, err := materializer.MaterializeBytesResult(context.Background(), 7, "task_noscope", "f.png", "", png, taskmedia.ResultMaterializeOptions{}); err == nil {
		t.Fatal("missing scope (bytes) = nil, want fail-closed")
	}
}

func TestWorkspaceProviderMaterializerIngestsInlineData(t *testing.T) {
	ctx := materializerTestScope(context.Background())
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	objects := &fakeRendererObjects{}
	materializer, err := taskruntime.NewWorkspaceProviderResultMaterializer(materializerTestDeps(media, objects))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	png := fixturePNGBytes(t)
	sum := fixtureChecksum(png)
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)

	results, err := materializer.MaterializeRemoteResults(ctx, 7, "task_inline1", []string{dataURL}, taskmedia.ResultMaterializeOptions{})
	if err != nil {
		t.Fatalf("MaterializeRemoteResults: %v", err)
	}
	if len(results.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(results.Items))
	}
	item := results.Items[0]
	if item.ImageURL == "" || item.Output.AssetID == "" || item.ImageURL != item.Output.AssetID {
		t.Fatalf("item = %+v, want asset-kind with matching envelope id", item)
	}
	if item.Output.URL != "" || item.Output.ChecksumSHA256 != "" {
		t.Fatalf("output = %+v, want id-only (no URL/checksum round-trip)", item.Output)
	}
	if item.Output.ResultIndex != 0 || item.Output.Role != taskruntime.WorkspaceProviderResultRole {
		t.Fatalf("output = %+v, want role/index carried", item.Output)
	}
	asset, err := media.GetAsset(ctx, "ws_test", item.Output.AssetID)
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if asset.MIMEType != "image/png" || asset.ChecksumSHA256 != sum || asset.SizeBytes != int64(len(png)) {
		t.Fatalf("asset = %+v, want validated byte facts", asset)
	}
	if len(objects.objects[asset.StorageKey]) != len(png) {
		t.Fatal("ingested bytes missing at the asset slot")
	}
}

func TestWorkspaceProviderMaterializerDownloadsRemote(t *testing.T) {
	ctx := materializerTestScope(context.Background())
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	objects := &fakeRendererObjects{}
	png := fixturePNGBytes(t)
	deps := materializerTestDeps(media, objects)
	deps.Fetcher = taskruntime.WorkspaceResultBytesFetchFunc(func(_ context.Context, task *workspacetasks.WorkspaceTask, ref workspacetasks.OutputRef) ([]byte, error) {
		if ref.URL != "https://cdn.example.com/fake-provider.png" {
			return nil, errors.New("fake: unexpected fetch URL")
		}
		return append([]byte(nil), png...), nil
	})
	materializer, err := taskruntime.NewWorkspaceProviderResultMaterializer(deps)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	results, err := materializer.MaterializeRemoteResults(ctx, 7, "task_remote1",
		[]string{"https://cdn.example.com/fake-provider.png"}, taskmedia.ResultMaterializeOptions{})
	if err != nil {
		t.Fatalf("MaterializeRemoteResults: %v", err)
	}
	if len(results.Items) != 1 || results.Items[0].Output.AssetID == "" {
		t.Fatalf("items = %+v, want one asset-kind output", results.Items)
	}
	if _, err := media.GetAsset(ctx, "ws_test", results.Items[0].Output.AssetID); err != nil {
		t.Fatalf("ingested asset missing: %v", err)
	}
}

func TestWorkspaceProviderMaterializerPassesAllowHostsToFetcher(t *testing.T) {
	ctx := materializerTestScope(context.Background())
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	objects := &fakeRendererObjects{}
	png := fixturePNGBytes(t)
	var got []string
	deps := materializerTestDeps(media, objects)
	deps.Fetcher = taskruntime.WorkspaceResultBytesFetchFunc(func(ctx context.Context, _ *workspacetasks.WorkspaceTask, _ workspacetasks.OutputRef) ([]byte, error) {
		got = taskruntime.WorkspaceResultAllowHostsFromCtx(ctx)
		return append([]byte(nil), png...), nil
	})
	m, err := taskruntime.NewWorkspaceProviderResultMaterializer(deps)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := m.MaterializeRemoteResults(ctx, 7, "task_hosts1", []string{"https://cdn.example.com/result.png"}, taskmedia.ResultMaterializeOptions{AllowHosts: []string{"cdn.example.com"}}); err != nil {
		t.Fatalf("MaterializeRemoteResults: %v", err)
	}
	if len(got) != 1 || got[0] != "cdn.example.com" {
		t.Fatalf("allow hosts = %v, want the executor policy", got)
	}
}

func TestWorkspaceProviderMaterializerRejectsBadBytes(t *testing.T) {
	ctx := materializerTestScope(context.Background())
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	materializer, err := taskruntime.NewWorkspaceProviderResultMaterializer(materializerTestDeps(media, &fakeRendererObjects{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	badPayload := base64.StdEncoding.EncodeToString([]byte("not-an-image-at-all"))
	if _, err := materializer.MaterializeRemoteResults(ctx, 7, "task_bad1",
		[]string{"data:image/png;base64," + badPayload}, taskmedia.ResultMaterializeOptions{}); err == nil {
		t.Fatal("non-image inline = nil, want imageformat failure")
	}
	if _, err := materializer.MaterializeRemoteResults(ctx, 7, "task_bad2",
		[]string{"data:image/png;base64,!!!not-base64!!!"}, taskmedia.ResultMaterializeOptions{}); err == nil {
		t.Fatal("malformed inline = nil, want decode failure")
	} else if strings.Contains(err.Error(), "!!!not-base64!!!") {
		t.Fatalf("error echoes payload: %v", err)
	}
	if _, err := materializer.MaterializeRemoteResults(ctx, 7, "task_bad3",
		[]string{}, taskmedia.ResultMaterializeOptions{}); err == nil {
		t.Fatal("empty sources = nil, want error")
	}
}

func TestWorkspaceProviderMaterializerBytesResult(t *testing.T) {
	ctx := materializerTestScope(context.Background())
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	materializer, err := taskruntime.NewWorkspaceProviderResultMaterializer(materializerTestDeps(media, &fakeRendererObjects{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	png := fixturePNGBytes(t)
	results, err := materializer.MaterializeBytesResult(ctx, 7, "task_bytes1", "result.png", "", png, taskmedia.ResultMaterializeOptions{})
	if err != nil {
		t.Fatalf("MaterializeBytesResult: %v", err)
	}
	if len(results.Items) != 1 || results.Items[0].Output.AssetID == "" {
		t.Fatalf("items = %+v, want one asset-kind output", results.Items)
	}
}
