package remote

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/platform/safehttp"
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

func DownloadImageWithLimit(ctx context.Context, remoteURL string, allowHosts []string, maxSize int64) (*ImageFile, error) {
	if maxSize <= 0 {
		maxSize = MaxRemoteFileSize
	}
	resp, err := safehttp.GetWithAllowHosts(ctx, remoteURL, 30*time.Second, allowHosts...)
	if err != nil {
		return nil, fmt.Errorf("下载图片失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("下载图片返回 %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "ideaboard-remote-image-*")
	if err != nil {
		return nil, fmt.Errorf("创建临时文件失败: %w", err)
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
		return nil, fmt.Errorf("读取图片数据失败: %w", err)
	}
	if size > maxSize {
		return nil, fmt.Errorf("远程文件超过大小限制 (%d MB)", maxSize/1024/1024)
	}

	facts, err := imageformat.InspectDetected(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("远程图片校验失败: %w", err)
	}
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("读取临时文件失败: %w", err)
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
