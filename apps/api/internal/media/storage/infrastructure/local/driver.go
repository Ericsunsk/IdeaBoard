package local

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/infrastructure/remote"
	"ideaboard/api/internal/media/storage/ports"
)

var localCopyBufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 32*1024)
		return &buf
	},
}

type LocalStorageProfileConfig struct{}

func (LocalStorageProfileConfig) Map() map[string]interface{} {
	return map[string]interface{}{}
}

type LocalStorageDriver struct {
	publicAPIOrigin string
	// root is the canonical filesystem root all object keys resolve under.
	// It is fixed once at construction so file layout never depends on the
	// mutable process working directory of the calling context.
	root string
}

var _ ports.StorageDriver = (*LocalStorageDriver)(nil)

// ResolveLocalStorageRoot canonicalizes the configured local storage root.
// An empty configured value falls back to the process working directory,
// resolved to an absolute path exactly once here instead of at every call.
func ResolveLocalStorageRoot(configured string) string {
	trimmed := strings.TrimSpace(configured)
	if trimmed == "" {
		if cwd, err := os.Getwd(); err == nil {
			trimmed = cwd
		}
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		absolute = filepath.Clean(trimmed)
	}
	return absolute
}

func NewLocalStorageDriver(publicAPIOrigin string) *LocalStorageDriver {
	return NewLocalStorageDriverWithRoot(publicAPIOrigin, "")
}

// NewLocalStorageDriverWithRoot binds the driver to a canonical storage root.
// Pass "" to resolve the process working directory once at construction.
func NewLocalStorageDriverWithRoot(publicAPIOrigin, root string) *LocalStorageDriver {
	return &LocalStorageDriver{
		publicAPIOrigin: strings.TrimRight(strings.TrimSpace(publicAPIOrigin), "/"),
		root:            ResolveLocalStorageRoot(root),
	}
}

// localPath joins a normalized object key onto the canonical root and
// rejects any result escaping the root (defense in depth: normalized keys
// already block traversal at the domain boundary).
func (d *LocalStorageDriver) localPath(objectKey string) (string, error) {
	full := filepath.Join(d.root, filepath.FromSlash(objectKey))
	relative, err := filepath.Rel(d.root, full)
	if err != nil {
		return "", fmt.Errorf(msgLocalPath, err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf(msgLocalPath2)
	}
	return full, nil
}

func (d *LocalStorageDriver) Provider() domain.StorageProvider {
	return domain.StorageProviderLocal
}

func (d *LocalStorageDriver) UsesObjectStorage() bool {
	return false
}

func (d *LocalStorageDriver) IsProfileReady(profile domain.StorageProfile) bool {
	return true
}

func (d *LocalStorageDriver) TestProfile(ctx context.Context, profile *domain.StorageProfile) error {
	return ctx.Err()
}

func (d *LocalStorageDriver) GetBucketInfo(ctx context.Context, profile *domain.StorageProfile) (map[string]interface{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf(msgGetBucketInfo)
}

func (d *LocalStorageDriver) UploadRef(ctx context.Context, profile *domain.StorageProfile, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	facts, err := imageformat.InspectFileHeader(file)
	if err != nil {
		return nil, fmt.Errorf(msgUploadRef, err)
	}
	filename := domain.CanonicalStorageFilename(file.Filename, facts.Extension)
	opts.ObjectKey = domain.CanonicalObjectKeyExtension(opts.ObjectKey, facts.Extension)

	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf(msgUploadRef2, err)
	}
	defer src.Close()

	ref, err := d.SaveReaderLocalRef(ctx, filename, facts.MIMEType, src, file.Size, opts)
	if err != nil {
		return nil, err
	}
	ref.Width = facts.Width
	ref.Height = facts.Height
	ref.Animated = facts.Animated
	return ref, nil
}

func (d *LocalStorageDriver) StoreBytesRef(ctx context.Context, profile *domain.StorageProfile, filename string, data []byte, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	facts, err := imageformat.InspectDetectedBytes(data)
	if err != nil {
		return nil, fmt.Errorf(msgStoreBytesRef, err)
	}
	filename = domain.CanonicalStorageFilename(filename, facts.Extension)
	opts.ObjectKey = domain.CanonicalObjectKeyExtension(opts.ObjectKey, facts.Extension)

	ref, err := d.SaveReaderLocalRef(ctx, filename, facts.MIMEType, bytes.NewReader(data), int64(len(data)), opts)
	if err != nil {
		return nil, err
	}
	ref.Width = facts.Width
	ref.Height = facts.Height
	ref.Animated = facts.Animated
	return ref, nil
}

func (d *LocalStorageDriver) StoreRemoteRef(ctx context.Context, profile *domain.StorageProfile, remoteURL string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	remoteFile, err := remote.DownloadImage(ctx, remoteURL, opts.AllowHosts)
	if err != nil {
		return nil, err
	}
	defer remoteFile.Cleanup()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opts.ObjectKey = domain.CanonicalObjectKeyExtension(opts.ObjectKey, filepath.Ext(remoteFile.Filename))
	ref, err := d.SaveReaderLocalRef(ctx, remoteFile.Filename, remoteFile.ContentType, remoteFile, remoteFile.Size, opts)
	if err != nil {
		return nil, err
	}
	ref.Width = remoteFile.Width
	ref.Height = remoteFile.Height
	ref.Animated = remoteFile.Animated
	return ref, nil
}

func (d *LocalStorageDriver) SaveReaderLocalRef(ctx context.Context, filename string, contentType string, src io.Reader, size int64, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = ".png"
	}
	objectKey := strings.TrimSpace(opts.ObjectKey)
	var err error
	if objectKey == "" {
		objectKey, err = domain.BuildObjectKey(opts.DirPrefix, ext)
	}
	if err != nil {
		return nil, err
	}
	objectKey, err = domain.NormalizeUploadObjectKey(objectKey)
	if err != nil {
		return nil, fmt.Errorf(msgSaveReaderLocalRef, err)
	}
	dst, err := d.localPath(objectKey)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return nil, fmt.Errorf(msgSaveReaderLocalRef2, err)
	}
	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf(msgSaveReaderLocalRef3, err)
	}
	cleanupDst := true
	defer func() {
		if cleanupDst {
			_ = os.Remove(dst)
		}
	}()
	hash := sha256.New()
	if err := copyExactWithBuffer(io.MultiWriter(dstFile, hash), src, size); err != nil {
		_ = dstFile.Close()
		return nil, fmt.Errorf(msgSaveReaderLocalRef3, err)
	}
	if err := dstFile.Close(); err != nil {
		return nil, fmt.Errorf(msgSaveReaderLocalRef3, err)
	}
	cleanupDst = false
	if strings.TrimSpace(contentType) == "" {
		contentType = inferStoredUploadContentType(filename, nil)
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	return &ports.StoredUploadRef{
		URL:            d.PublicUploadURL(objectKey),
		StorageKey:     objectKey,
		StorageProfile: "builtin-local",
		MIMEType:       contentType,
		SizeBytes:      size,
		ChecksumSHA256: hex.EncodeToString(hash.Sum(nil)),
		Animated:       strings.EqualFold(ext, ".gif"),
	}, nil
}

func (d *LocalStorageDriver) PutObject(ctx context.Context, profile *domain.StorageProfile, objectKey string, data []byte, contentType string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	normalized, err := domain.NormalizeUploadObjectKey(objectKey)
	if err != nil {
		return err
	}
	dst, err := d.localPath(normalized)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return fmt.Errorf(msgSaveReaderLocalRef2, err)
	}
	tmpDst := fmt.Sprintf("%s.tmp.%d", dst, time.Now().UnixNano())
	if err := os.WriteFile(tmpDst, data, 0600); err != nil {
		return fmt.Errorf(msgSaveReaderLocalRef3, err)
	}
	if err := os.Rename(tmpDst, dst); err != nil {
		_ = os.Remove(tmpDst)
		return fmt.Errorf(msgSaveReaderLocalRef3, err)
	}
	return nil
}

func (d *LocalStorageDriver) Read(ctx context.Context, profile *domain.StorageProfile, objectKey string, maxBytes int64) (*ports.StoredUploadObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	normalized, err := domain.NormalizeUploadObjectKey(objectKey)
	if err != nil {
		return nil, err
	}
	localPath, err := d.localPath(normalized)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, domain.ErrStoredObjectNotFound
		}
		return nil, fmt.Errorf(msgRead, err)
	}
	defer file.Close()
	data, err := readStorageObject(file, maxBytes)
	if err != nil {
		return nil, err
	}
	return &ports.StoredUploadObject{
		ObjectKey:   normalized,
		Filename:    filepath.Base(normalized),
		ContentType: inferStoredUploadContentType(filepath.Base(normalized), data),
		Data:        data,
	}, nil
}

func (d *LocalStorageDriver) Delete(ctx context.Context, profile *domain.StorageProfile, objectKey string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	normalized, err := domain.NormalizeUploadObjectKey(objectKey)
	if err != nil {
		return err
	}
	localPath, err := d.localPath(normalized)
	if err != nil {
		return err
	}
	if err := os.Remove(localPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf(msgDelete, err)
	}
	d.cleanupEmptyParentDirs(localPath)
	return nil
}

func (d *LocalStorageDriver) DeleteByPrefix(ctx context.Context, profile *domain.StorageProfile, objectKeyPrefix string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	normalizedPrefix, err := domain.NormalizeUploadObjectPrefix(objectKeyPrefix)
	if err != nil {
		return err
	}
	localPath, err := d.localPath(strings.TrimSuffix(normalizedPrefix, "/"))
	if err != nil {
		return err
	}
	if err := os.RemoveAll(localPath); err != nil {
		return fmt.Errorf(msgDeleteByPrefix, err)
	}
	d.cleanupEmptyParentDirs(localPath)
	return nil
}

func (d *LocalStorageDriver) ListObjectKeysByPrefix(ctx context.Context, profile *domain.StorageProfile, objectKeyPrefix string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Single-source traversal defense: the domain normalizer anchors the
	// prefix under uploads/ or ws-uploads/ (mirroring DeleteByPrefix); the
	// driver-local localPath root check stays as defense in depth.
	normalizedPrefix, err := domain.NormalizeUploadObjectPrefix(objectKeyPrefix)
	if err != nil {
		return nil, err
	}
	root, err := d.localPath(strings.TrimSuffix(normalizedPrefix, "/"))
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.SkipDir
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(d.root, path)
		if err != nil {
			return err
		}
		keys = append(keys, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func (d *LocalStorageDriver) SignedURL(_ context.Context, _ *domain.StorageProfile, _ string, _ time.Duration) (string, error) {
	return "", domain.ErrProviderHTTPSUnavailable
}

func (d *LocalStorageDriver) ProviderURL(_ *domain.StorageProfile, _ string) (string, error) {
	return "", domain.ErrProviderHTTPSUnavailable
}

func (d *LocalStorageDriver) cleanupEmptyParentDirs(localPath string) {
	dir := filepath.Clean(filepath.Dir(localPath))
	for dir != d.root && strings.HasPrefix(dir, d.root) {
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func (d *LocalStorageDriver) VerifyObjectGeneration(ctx context.Context, profile *domain.StorageProfile, objectKey string, expected int64) error {
	normalized, err := domain.NormalizeUploadObjectKey(objectKey)
	if err != nil {
		return err
	}
	localPath, err := d.localPath(normalized)
	if err != nil {
		return err
	}
	if _, err := os.Stat(localPath); err != nil {
		if os.IsNotExist(err) {
			return domain.ErrStoredObjectNotFound
		}
		return err
	}
	return nil
}

func (d *LocalStorageDriver) PublicUploadURL(objectKey string) string {
	normalized := "/" + strings.TrimLeft(filepath.ToSlash(objectKey), "/")
	if d.publicAPIOrigin != "" {
		return d.publicAPIOrigin + normalized
	}
	return normalized
}

type localCopyWriter struct {
	dst io.Writer
}

func (w localCopyWriter) Write(p []byte) (int, error) {
	return w.dst.Write(p)
}

func copyExactWithBuffer(dst io.Writer, src io.Reader, size int64) error {
	bufPtr := localCopyBufferPool.Get().(*[]byte)
	defer localCopyBufferPool.Put(bufPtr)

	if size < 0 {
		_, err := io.CopyBuffer(localCopyWriter{dst: dst}, src, *bufPtr)
		return err
	}

	written, err := io.CopyBuffer(localCopyWriter{dst: dst}, io.LimitReader(src, size), *bufPtr)
	if err != nil {
		return err
	}
	if written != size {
		return io.EOF
	}
	return nil
}

func readStorageObject(src io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 || maxBytes > remote.MaxRemoteFileSize {
		maxBytes = remote.MaxRemoteFileSize
	}
	data, err := io.ReadAll(io.LimitReader(src, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf(msgRead, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf(msgReadStorageObject, maxBytes/1024/1024)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf(msgReadStorageObject2)
	}
	return data, nil
}

func inferStoredUploadContentType(filename string, data []byte) string {
	if len(data) > 0 {
		if ext := imageformat.DetectImageExtensionFromBytes(data); ext != "" {
			return imageformat.ImageMIMEFromExtension(ext)
		}
	}
	return imageformat.ImageMIMEFromExtension(filepath.Ext(filename))
}
