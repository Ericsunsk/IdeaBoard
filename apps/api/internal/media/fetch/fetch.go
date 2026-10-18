// Package fetch owns the bytes behind one allowlisted media URL. It is
// the single owner of the download-to-bytes logic previously duplicated
// in the library materializer and the workspace task result fetcher:
// allowlisted download via the shared remote downloader (SSRF guard,
// timeout, size cap) followed by a bounded in-memory read. Callers own
// host allowlisting (see media/egress) and ingestion; this package never
// persists anything.
package fetch

import (
	"context"
	"fmt"
	"io"
	"strings"

	"ideaboard/api/internal/media/storage/infrastructure/remote"
)

// BytesFetcher downloads the bytes behind one allowlisted URL.
// Implementations own allowlisting, timeouts, and size caps; tests inject
// canned fixture bytes.
type BytesFetcher interface {
	FetchBytes(ctx context.Context, rawURL string, allowHosts []string) ([]byte, error)
}

// FetchBytes downloads one URL to bytes through the shared remote
// downloader and returns the bounded in-memory copy. maxBytes bounds both
// the transport and the read; payloads larger than maxBytes fail closed.
func FetchBytes(ctx context.Context, rawURL string, allowHosts []string, maxBytes int64) ([]byte, error) {
	file, err := remote.DownloadImageWithLimit(ctx, strings.TrimSpace(rawURL), allowHosts, maxBytes)
	if err != nil {
		return nil, err
	}
	defer file.Cleanup()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind remote image: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read remote image: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("remote image exceeds size limit")
	}
	return data, nil
}
