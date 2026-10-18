package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"io"
	"mime/multipart"
	"net/textproto"
	"path/filepath"
	"strings"
	"time"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/media/storage/application/profiles"
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
)

const maxRemoteFileSize = 50 * 1024 * 1024

type Service struct {
	profiles        *profiles.Service
	drivers         profiles.DriverResolver
	publicAPIOrigin string
}

func NewService(
	profiles *profiles.Service,
	drivers profiles.DriverResolver,
	publicAPIOrigin string,
) *Service {
	return &Service{
		profiles:        profiles,
		drivers:         drivers,
		publicAPIOrigin: strings.TrimRight(strings.TrimSpace(publicAPIOrigin), "/"),
	}
}

func (s *Service) Profiles() *profiles.Service {
	return s.profiles
}

func (s *Service) Upload(ctx context.Context, file *multipart.FileHeader) (string, error) {
	return s.UploadWithOptions(ctx, file, ports.StorageStoreOptions{})
}

func (s *Service) UploadWithOptions(ctx context.Context, file *multipart.FileHeader, opts ports.StorageStoreOptions) (string, error) {
	ref, err := s.UploadRefWithOptions(ctx, file, opts)
	if err != nil {
		return "", err
	}
	return ref.URL, nil
}

func (s *Service) UploadRefWithOptions(ctx context.Context, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	return s.UploadRefWithProfile(ctx, *profile, file, opts)
}

func (s *Service) UploadRefAtProfile(ctx context.Context, profileID string, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveProfileByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf(errorrender.MsgUploadRefAtProfile, err)
	}
	return s.UploadRefWithProfile(ctx, *profile, file, opts)
}

func (s *Service) PrepareUploadRefFromFile(ctx context.Context, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	facts, err := canonicalizeMultipartImageWithFacts(file)
	if err != nil {
		return nil, err
	}
	checksum, err := checksumMultipartFile(file)
	if err != nil {
		return nil, fmt.Errorf(errorrender.MsgPrepareUploadRefFromFile, err)
	}
	key := strings.TrimSpace(opts.ObjectKey)
	if key == "" {
		key, err = domain.BuildObjectKey(opts.DirPrefix, facts.Extension)
		if err != nil {
			return nil, err
		}
	} else {
		key = domain.CanonicalObjectKeyExtension(key, facts.Extension)
	}
	key, err = domain.NormalizeUploadObjectKey(key)
	if err != nil {
		return nil, fmt.Errorf(errorrender.MsgPrepareUploadRefFromFile2, err)
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return nil, err
	}
	publicURL, urlErr := driver.ProviderURL(profile, key)
	if urlErr != nil && profile.Provider == domain.StorageProviderLocal {
		publicURL = s.PublicUploadURL(key)
		urlErr = nil
	}
	if urlErr != nil {
		return nil, urlErr
	}
	return &ports.StoredUploadRef{
		URL:            publicURL,
		StorageKey:     key,
		StorageProfile: profile.ID,
		MIMEType:       facts.MIMEType,
		Width:          facts.Width,
		Height:         facts.Height,
		SizeBytes:      file.Size,
		ChecksumSHA256: checksum,
		Animated:       facts.Animated,
	}, nil
}

func checksumMultipartFile(file *multipart.FileHeader) (string, error) {
	if file == nil {
		return "", errors.New(errorrender.MsgChecksumMultipartFile)
	}
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, src); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *Service) PrepareUploadRef(ctx context.Context, filename string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(opts.ObjectKey)
	if key == "" {
		key, err = domain.BuildObjectKey(opts.DirPrefix, filepath.Ext(filename))
		if err != nil {
			return nil, err
		}
	}
	key, err = domain.NormalizeUploadObjectKey(key)
	if err != nil {
		return nil, fmt.Errorf(errorrender.MsgPrepareUploadRefFromFile2, err)
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return nil, err
	}
	publicURL, urlErr := driver.ProviderURL(profile, key)
	if urlErr != nil && profile.Provider == domain.StorageProviderLocal {
		publicURL = s.PublicUploadURL(key)
		urlErr = nil
	}
	if urlErr != nil {
		return nil, urlErr
	}
	return &ports.StoredUploadRef{
		URL:            publicURL,
		StorageKey:     key,
		StorageProfile: profile.ID,
		MIMEType:       inferStoredUploadContentType(filename, nil),
	}, nil
}

func (s *Service) StoreRemoteRef(ctx context.Context, remoteURL string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	return s.StoreRemoteRefWithProfile(ctx, *profile, remoteURL, opts)
}

func (s *Service) StoreMediaRef(ctx context.Context, sourceRef string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	return s.StoreMediaRefWithProfile(ctx, *profile, sourceRef, opts)
}

func (s *Service) StoreMediaRefAtProfile(ctx context.Context, profileID, sourceRef string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveProfileByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf(errorrender.MsgUploadRefAtProfile, err)
	}
	return s.StoreMediaRefWithProfile(ctx, *profile, sourceRef, opts)
}

func (s *Service) StoreMediaRefWithProfile(ctx context.Context, profile domain.StorageProfile, sourceRef string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	trimmed := strings.TrimSpace(sourceRef)
	if strings.HasPrefix(trimmed, "data:") {
		filename, data, err := decodeDataURL(trimmed)
		if err != nil {
			return nil, err
		}
		return s.StoreBytesRefWithProfile(ctx, profile, filename, data, opts)
	}
	return s.StoreRemoteRefWithProfile(ctx, profile, trimmed, opts)
}

func (s *Service) StoreBytesRef(ctx context.Context, filename string, data []byte, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveActiveProfile(ctx)
	if err != nil {
		return nil, err
	}
	return s.StoreBytesRefWithProfile(ctx, *profile, filename, data, opts)
}

func (s *Service) StoreBytesRefAtProfile(ctx context.Context, profileID, filename string, data []byte, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveProfileByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf(errorrender.MsgUploadRefAtProfile, err)
	}
	return s.StoreBytesRefWithProfile(ctx, *profile, filename, data, opts)
}

func (s *Service) StoreBytesRefWithProfile(ctx context.Context, profile domain.StorageProfile, filename string, data []byte, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return nil, err
	}
	return driver.StoreBytesRef(ctx, &profile, filename, data, opts)
}

func (s *Service) StoreRemoteRefAtProfile(ctx context.Context, profileID, remoteURL string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	profile, err := s.profiles.ResolveProfileByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf(errorrender.MsgUploadRefAtProfile, err)
	}
	return s.StoreRemoteRefWithProfile(ctx, *profile, remoteURL, opts)
}

func (s *Service) StoreRemoteRefWithProfile(ctx context.Context, profile domain.StorageProfile, remoteURL string, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return nil, err
	}
	return driver.StoreRemoteRef(ctx, &profile, remoteURL, opts)
}

func (s *Service) UploadRefWithProfile(ctx context.Context, profile domain.StorageProfile, file *multipart.FileHeader, opts ports.StorageStoreOptions) (*ports.StoredUploadRef, error) {
	facts, err := canonicalizeMultipartImageWithFacts(file)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.ObjectKey) != "" {
		opts.ObjectKey = domain.CanonicalObjectKeyExtension(opts.ObjectKey, facts.Extension)
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return nil, err
	}
	return driver.UploadRef(ctx, &profile, file, opts)
}

func (s *Service) PutObjectAtProfile(ctx context.Context, profileID, objectKey string, data []byte, contentType string) error {
	profile, err := s.profiles.ResolveProfileByID(ctx, profileID)
	if err != nil {
		return fmt.Errorf(errorrender.MsgPutObjectAtProfile, err)
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return err
	}
	return driver.PutObject(ctx, profile, objectKey, data, contentType)
}

func (s *Service) ReadObjectAtProfile(ctx context.Context, profileID, objectKey string, maxBytes int64) (*ports.StoredUploadObject, error) {
	profile, err := s.profiles.ResolveProfileByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf(errorrender.MsgPutObjectAtProfile, err)
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return nil, err
	}
	return driver.Read(ctx, profile, objectKey, maxBytes)
}

func (s *Service) DeleteUserUploads(ctx context.Context, userID int64, storageUID string) error {
	prefixes, err := domain.UserUploadObjectPrefixes(userID, storageUID)
	if err != nil {
		return err
	}
	return s.DeleteUploadObjectPrefixes(ctx, prefixes)
}

func (s *Service) DeleteObjectByKey(ctx context.Context, objectKey string) error {
	profile, err := s.profiles.ResolveActiveProfile(ctx)
	if err != nil {
		return err
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return err
	}
	return driver.Delete(ctx, profile, objectKey)
}

func (s *Service) DeleteObjectByStorageProfile(ctx context.Context, storageProfileID string, objectKey string) error {
	profile, err := s.profiles.ResolveProfileByID(ctx, storageProfileID)
	if err != nil {
		return err
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return err
	}
	return driver.Delete(ctx, profile, objectKey)
}

func (s *Service) ListObjectKeysByStorageProfile(ctx context.Context, storageProfileID string, objectKeyPrefix string) ([]string, error) {
	profile, err := s.profiles.ResolveProfileByID(ctx, storageProfileID)
	if err != nil {
		return nil, err
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return nil, err
	}
	return driver.ListObjectKeysByPrefix(ctx, profile, objectKeyPrefix)
}

func (s *Service) DeleteUploadObjectPrefix(ctx context.Context, prefix string) error {
	return s.DeleteUploadObjectPrefixes(ctx, []string{prefix})
}

func (s *Service) DeleteUploadObjectPrefixes(ctx context.Context, prefixes []string) error {
	profiles, err := s.profiles.LoadProfilesWithSecrets(ctx)
	if err != nil {
		return err
	}
	var firstErr error
	for _, prefix := range prefixes {
		if strings.TrimSpace(prefix) == "" {
			continue
		}
		for _, profile := range profiles {
			driver, err := s.drivers.GetDriver(profile.Provider)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			if err := driver.DeleteByPrefix(ctx, &profile, prefix); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (s *Service) UsesObjectStorage(ctx context.Context) bool {
	profile, err := s.profiles.ResolveActiveProfile(ctx)
	if err != nil {
		return false
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return false
	}
	return driver.UsesObjectStorage()
}

func canonicalizeMultipartImageWithFacts(file *multipart.FileHeader) (imageformat.Facts, error) {
	facts, err := imageformat.InspectFileHeader(file)
	if err != nil {
		return imageformat.Facts{}, fmt.Errorf(errorrender.MsgCanonicalizeMultipartImageWithFacts, err)
	}
	if file == nil {
		return imageformat.Facts{}, errors.New(errorrender.MsgChecksumMultipartFile)
	}
	file.Filename = domain.CanonicalStorageFilename(file.Filename, facts.Extension)
	if file.Header == nil {
		file.Header = make(textproto.MIMEHeader)
	}
	file.Header.Set("Content-Type", facts.MIMEType)
	return facts, nil
}

func (s *Service) PublicUploadURL(objectKey string) string {
	normalized := "/" + strings.TrimLeft(filepath.ToSlash(objectKey), "/")
	if s.publicAPIOrigin != "" {
		return s.publicAPIOrigin + normalized
	}
	return normalized
}

func decodeDataURL(dataURL string) (string, []byte, error) {
	// Thin adapter over the single data-URL seam mediarefs.ParseDataImageURL
	// (shared with the provider-result inline path); filename minting stays
	// local to the upload path.
	parsed, err := mediarefs.ParseDataImageURL(dataURL, maxRemoteFileSize)
	if err != nil {
		return "", nil, errors.New(errorrender.MsgDecodeDataURL)
	}
	return fmt.Sprintf("gen_%d%s", time.Now().UnixNano(), parsed.Ext), parsed.Data, nil
}

func inferStoredUploadContentType(filename string, data []byte) string {
	if len(data) > 0 {
		if ext := imageformat.DetectImageExtensionFromBytes(data); ext != "" {
			return imageformat.ImageMIMEFromExtension(ext)
		}
	}
	return imageformat.ImageMIMEFromExtension(filepath.Ext(filename))
}
