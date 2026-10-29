package imageformat

import (
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"io"
	"mime/multipart"
)

// ErrUnsupportedImageFormat reports bytes whose sniffed format is unknown
// to the upload allowlist (heic, svg, truncated headers, …) or a claimed
// extension the registry does not know. Edges map it to 415
// (CodeMediaUnsupportedType); mismatched-but-known formats stay 422 via
// MsgValidateImageMagic.
var ErrUnsupportedImageFormat = errors.New("imageformat: unsupported image format")

// InspectFileHeader determines the image format from the uploaded bytes and
// validates its dimensions. The filename extension is deliberately ignored:
// multipart metadata is user-controlled and may be stale after content
// negotiation or a client-side rename. The single Decode truth is
// image.DecodeConfig inside Inspect (which runs the registered format
// decoder, including the AVIF config decoder): no second full pixel decode
// runs here, so every accepted format validates at identical depth.
// Unknown formats fail with ErrUnsupportedImageFormat (edges map to 415).
func InspectFileHeader(fh *multipart.FileHeader) (Facts, error) {
	if fh == nil {
		return Facts{}, fmt.Errorf("%w: %s", ErrUnsupportedImageFormat, errorrender.MsgInspectDetectedBytes)
	}
	f, err := fh.Open()
	if err != nil {
		return Facts{}, fmt.Errorf(errorrender.MsgInspectFileHeader, err)
	}
	defer f.Close()
	return InspectDetected(f)
}

// InspectDetected detects the canonical format from the source bytes before
// running the normal signature and image-config validation.
func InspectDetected(source io.ReadSeeker) (Facts, error) {
	if source == nil {
		return Facts{}, errors.New(errorrender.MsgInspectDetectedBytes)
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return Facts{}, fmt.Errorf(errorrender.MsgInspect3, err)
	}
	header := make([]byte, 64*1024)
	n, readErr := io.ReadFull(source, header)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return Facts{}, fmt.Errorf(errorrender.MsgInspect3, readErr)
	}
	ext := DetectImageExtensionFromBytes(header[:n])
	if ext == "" {
		return Facts{}, fmt.Errorf("%w: %s", ErrUnsupportedImageFormat, errorrender.MsgInspectDetectedBytes)
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return Facts{}, fmt.Errorf(errorrender.MsgInspect3, err)
	}
	return Inspect(source, ext)
}

// sniffWindowBytes bounds the single shared-database sniff on streams:
// 512 bytes cover every allowlisted magic (PNG needs 8, GIF 6, WebP 16)
// without materializing the whole file.
const sniffWindowBytes = 512

// ValidateImageBytes verifies in-memory bytes match the allowed image extension.
func ValidateImageBytes(data []byte, ext string) error {
	return validateImageMagic(data, ext)
}

// ValidateImageReader verifies the image signature from a stream without
// requiring callers to materialize the whole file in memory: only the
// sniff window is buffered.
func ValidateImageReader(r io.Reader, ext string) error {
	normalizedExt, ok := NormalizeImageExtension(ext)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnsupportedImageFormat, fmt.Sprintf(errorrender.MsgInspect, ext))
	}
	if r == nil {
		return fmt.Errorf("%w: %s", ErrUnsupportedImageFormat, errorrender.MsgInspectDetectedBytes)
	}
	buf := make([]byte, sniffWindowBytes)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return fmt.Errorf("%w: %s", ErrUnsupportedImageFormat, errorrender.MsgInspectDetectedBytes)
	}
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrUnsupportedImageFormat, errorrender.MsgInspectDetectedBytes)
	}
	return validateImageMagic(buf[:n], normalizedExt)
}

func validateImageMagic(buf []byte, ext string) error {
	normalizedExt, ok := NormalizeImageExtension(ext)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnsupportedImageFormat, fmt.Sprintf(errorrender.MsgInspect, ext))
	}
	// Single sniff: the shared magic database decides. A different
	// allowlisted format is a mismatch (422); an unknown one (heic,
	// svg, truncated headers, …) can never satisfy the claimed extension
	// and reports unsupported (415 at the edges). There is no
	// allowlist-scoped per-format fallback.
	if detected := DetectImageExtensionFromBytes(buf); detected != "" {
		if detected != normalizedExt {
			return errors.New(errorrender.MsgValidateImageMagic)
		}
		return nil
	}
	return fmt.Errorf("%w: %s", ErrUnsupportedImageFormat, errorrender.MsgInspectDetectedBytes)
}
