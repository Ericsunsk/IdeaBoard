package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/platform/outboundhttp"
)

const MaxRemoteFileSize = 50 * 1024 * 1024

var copyBufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 32*1024)
		return &buf
	},
}

type ImageFile struct {
	Filename    string
	ContentType string
	Width       int
	Height      int
	Animated    bool
	Size        int64
	File        *os.File
	Path        string
}

func (f *ImageFile) Read(p []byte) (int, error) {
	return f.File.Read(p)
}

func (f *ImageFile) Seek(offset int64, whence int) (int64, error) {
	return f.File.Seek(offset, whence)
}

func (f *ImageFile) Cleanup() error {
	if f == nil {
		return nil
	}
	var closeErr error
	if f.File != nil {
		closeErr = f.File.Close()
		f.File = nil
	}
	var removeErr error
	if f.Path != "" {
		removeErr = os.Remove(f.Path)
		f.Path = ""
	}
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}

func DownloadImage(ctx context.Context, remoteURL string, allowHosts []string) (*ImageFile, error) {
	return DownloadImageWithLimit(ctx, remoteURL, allowHosts, MaxRemoteFileSize)
}

// remoteImageOutbound is the shared standard outbound client for remote
// image fetches: timeout cap, bounded retries and Retry-After respect match
// the gateway migration (#766).
var remoteImageOutbound = outboundhttp.NewClient(outboundhttp.Config{})

// remoteImageTarget names remote image egress in outbound logs/metrics.
const remoteImageTarget = "remote-image"

// remoteImageResponse fetches one remote URL through the standard outbound
// client. An exhausted retryable status returns its status with a nil
// error, exactly like the single-shot response it replaces, so the
// status-to-error mapping below keeps working. Only transport/timeout
// failures surface as errors.
func remoteImageResponse(ctx context.Context, remoteURL string, opts outboundhttp.RequestOptions) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
	if err != nil {
		return nil, err
	}
	opts.Timeout = outboundhttp.DefaultTimeout
	opts.Target = remoteImageTarget
	resp, err := remoteImageOutbound.Do(ctx, req, opts)
	if err != nil {
		var statusErr *outboundhttp.HTTPStatusError
		if errors.As(err, &statusErr) {
			preview := statusErr.Body
			return &http.Response{
				Status:        fmt.Sprintf("%d %s", statusErr.StatusCode, http.StatusText(statusErr.StatusCode)),
				StatusCode:    statusErr.StatusCode,
				Header:        make(http.Header),
				Body:          io.NopCloser(strings.NewReader(preview)),
				ContentLength: int64(len(preview)),
				Request:       req,
			}, nil
		}
		return nil, err
	}
	return resp, nil
}

func DownloadImageWithLimit(ctx context.Context, remoteURL string, allowHosts []string, maxSize int64) (*ImageFile, error) {
	return downloadImage(ctx, remoteURL, outboundhttp.RequestOptions{AllowHosts: allowHosts}, maxSize)
}

// DownloadPublicImage restricts preview fetches to public HTTPS provider hosts on every hop.
func DownloadPublicImage(ctx context.Context, remoteURL string, hosts []string, maxSize int64) (*ImageFile, error) {
	if len(hosts) == 0 {
		return nil, errors.New("public image hosts are required")
	}
	return downloadImage(ctx, remoteURL, outboundhttp.RequestOptions{PublicHosts: hosts}, maxSize)
}

func downloadImage(ctx context.Context, remoteURL string, opts outboundhttp.RequestOptions, maxSize int64) (*ImageFile, error) {
	if maxSize <= 0 {
		maxSize = MaxRemoteFileSize
	}
	resp, err := remoteImageResponse(ctx, remoteURL, opts)
	if err != nil {
		return nil, fmt.Errorf(msgDownloadImageWithLimit, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf(msgDownloadImageWithLimit2, resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "ideaboard-remote-image-*")
	if err != nil {
		return nil, fmt.Errorf(msgDownloadImageWithLimit3, err)
	}
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = tmpFile.Close()
			_ = os.Remove(tmpFile.Name())
		}
	}()

	size, err := copyWithBuffer(tmpFile, io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf(msgDownloadImageWithLimit4, err)
	}
	if size > maxSize {
		return nil, fmt.Errorf(msgDownloadImageWithLimit5, maxSize/1024/1024)
	}

	facts, err := imageformat.InspectDetected(tmpFile)
	if err != nil {
		return nil, fmt.Errorf(msgDownloadImageWithLimit6, err)
	}
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf(msgDownloadImageWithLimit7, err)
	}

	filename := fmt.Sprintf("remote_%d%s", time.Now().UnixNano(), facts.Extension)
	cleanupTemp = false
	return &ImageFile{
		Filename:    filename,
		ContentType: facts.MIMEType,
		Width:       facts.Width,
		Height:      facts.Height,
		Animated:    facts.Animated,
		Size:        size,
		File:        tmpFile,
		Path:        tmpFile.Name(),
	}, nil
}

type copyWriter struct {
	dst io.Writer
}

func (w copyWriter) Write(p []byte) (int, error) {
	return w.dst.Write(p)
}

func copyWithBuffer(dst io.Writer, src io.Reader) (int64, error) {
	bufPtr := copyBufferPool.Get().(*[]byte)
	defer copyBufferPool.Put(bufPtr)
	return io.CopyBuffer(copyWriter{dst: dst}, src, *bufPtr)
}
