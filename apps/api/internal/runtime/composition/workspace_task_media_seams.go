package composition

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"strings"

	"ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/derivatives/ports"
	"ideaboard/api/internal/media/storage/infrastructure/remote"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/tasking/workspacetasks"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	_ "image/jpeg"
)

// workspaceTaskResultBytesFetcher is the production remote-result seam. The
// downloader owns SSRF protection, outbound timeout and response size caps;
// this adapter only turns its temporary validated image file into bytes.
type workspaceTaskResultBytesFetcher struct{}

var _ WorkspaceResultBytesFetcher = workspaceTaskResultBytesFetcher{}

type workspaceResultAllowHostsKey struct{}

func withWorkspaceResultAllowHosts(ctx context.Context, hosts []string) context.Context {
	return context.WithValue(ctx, workspaceResultAllowHostsKey{}, append([]string(nil), hosts...))
}

func workspaceResultAllowHostsFromCtx(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	hosts, _ := ctx.Value(workspaceResultAllowHostsKey{}).([]string)
	return append([]string(nil), hosts...)
}

func (workspaceTaskResultBytesFetcher) FetchResultBytes(ctx context.Context, _ *workspacetasks.WorkspaceTask, ref workspacetasks.OutputRef) ([]byte, error) {
	allowHosts := workspaceResultAllowHostsFromCtx(ctx)
	if len(allowHosts) == 0 {
		allowHosts = outbound.AllowHosts(ctx)
	}
	file, err := remote.DownloadImageWithLimit(ctx, strings.TrimSpace(ref.URL), allowHosts, workspacemedia.MaxUploadSizeBytes)
	if err != nil {
		return nil, err
	}
	defer file.Cleanup()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind remote image: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(file, workspacemedia.MaxUploadSizeBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read remote image: %w", err)
	}
	if int64(len(data)) > workspacemedia.MaxUploadSizeBytes {
		return nil, fmt.Errorf("remote image exceeds size limit")
	}
	return data, nil
}

// workspaceTaskDerivativeRenderer is the bounded production renderer used by
// EnsureDerivatives. All output is normalized to PNG so derivative storage has
// a deterministic MIME type independent of the source format.
type workspaceTaskDerivativeRenderer struct{}

var _ ports.Renderer = workspaceTaskDerivativeRenderer{}

func (workspaceTaskDerivativeRenderer) Render(ctx context.Context, data []byte, sourceName string, maxSide int) ([]byte, int, int, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, 0, "", err
	}
	if len(data) == 0 {
		return nil, 0, 0, "", fmt.Errorf("empty derivative source")
	}
	if len(data) > domain.MaxSourceBytes {
		return nil, 0, 0, "", fmt.Errorf("derivative source exceeds size limit")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, "", fmt.Errorf("decode %s: %w", sourceName, err)
	}
	bounds := src.Bounds()
	sw, sh := bounds.Dx(), bounds.Dy()
	if sw <= 0 || sh <= 0 {
		return nil, 0, 0, "", fmt.Errorf("invalid source dimensions")
	}
	if maxSide <= 0 {
		maxSide = sw
		if sh > maxSide {
			maxSide = sh
		}
	}
	dw, dh := sw, sh
	if sw > maxSide || sh > maxSide {
		scale := float64(maxSide) / float64(sw)
		if sh > sw {
			scale = float64(maxSide) / float64(sh)
		}
		dw = max(1, int(float64(sw)*scale))
		dh = max(1, int(float64(sh)*scale))
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, dst); err != nil {
		return nil, 0, 0, "", fmt.Errorf("encode derivative: %w", err)
	}
	if encoded.Len() == 0 || encoded.Len() > domain.MaxOutputBytes {
		return nil, 0, 0, "", fmt.Errorf("derivative output exceeds size limit")
	}
	return encoded.Bytes(), dw, dh, "image/png", nil
}
