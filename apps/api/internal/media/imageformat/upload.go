package imageformat

import (
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"image"
	"io"
	"mime/multipart"
)

// InspectFileHeader determines the image format from the uploaded bytes and
// validates its dimensions. The filename extension is deliberately ignored:
// multipart metadata is user-controlled and may be stale after content
// negotiation or a client-side rename.
func InspectFileHeader(fh *multipart.FileHeader) (Facts, error) {
	if fh == nil {
		return Facts{}, errors.New(errorrender.MsgInspectDetectedBytes)
	}
	f, err := fh.Open()
	if err != nil {
		return Facts{}, fmt.Errorf(errorrender.MsgInspectFileHeader, err)
	}
	defer f.Close()
	facts, err := InspectDetected(f)
	if err != nil {
		return Facts{}, err
	}
	// Go does not currently ship an AVIF pixel decoder in this service. Its
	// container and dimension validation remains authoritative until a native
	// decoder is introduced; all other accepted upload formats are fully decoded.
	if facts.Extension != ".avif" {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return Facts{}, fmt.Errorf(errorrender.MsgInspect3, err)
		}
		if _, _, err := image.Decode(f); err != nil {
			return Facts{}, fmt.Errorf(errorrender.MsgInspectFileHeader2, err)
		}
	}
	return facts, nil
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
		return Facts{}, errors.New(errorrender.MsgInspectDetectedBytes)
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return Facts{}, fmt.Errorf(errorrender.MsgInspect3, err)
	}
	return Inspect(source, ext)
}

// ValidateImageBytes verifies in-memory bytes match the allowed image extension.
func ValidateImageBytes(data []byte, ext string) error {
	normalizedExt, ok := NormalizeImageExtension(ext)
	if !ok {
		return fmt.Errorf(errorrender.MsgInspect, ext)
	}

	signatures := imageSignaturesByExtension(normalizedExt)
	required := requiredImageHeaderLength(signatures)
	if len(data) < required {
		return errors.New(errorrender.MsgInspectDetectedBytes)
	}

	return validateImageMagic(data[:required], normalizedExt)
}

// ValidateImageReader verifies the image signature from a stream without
// requiring callers to materialize the whole file in memory.
func ValidateImageReader(r io.Reader, ext string) error {
	normalizedExt, ok := NormalizeImageExtension(ext)
	if !ok {
		return fmt.Errorf(errorrender.MsgInspect, ext)
	}
	if r == nil {
		return errors.New(errorrender.MsgInspectDetectedBytes)
	}

	signatures := imageSignaturesByExtension(normalizedExt)
	required := requiredImageHeaderLength(signatures)
	buf := make([]byte, required)
	n, err := io.ReadFull(r, buf)
	if err != nil || n < required {
		return errors.New(errorrender.MsgInspectDetectedBytes)
	}

	return validateImageMagic(buf, normalizedExt)
}

func requiredImageHeaderLength(signatures []imageSignature) int {
	maxLen := 0
	for _, sig := range signatures {
		if sig.offset+len(sig.bytes) > maxLen {
			maxLen = sig.offset + len(sig.bytes)
		}
	}
	return maxLen
}

func validateImageMagic(buf []byte, ext string) error {
	normalizedExt, ok := NormalizeImageExtension(ext)
	if !ok {
		return fmt.Errorf(errorrender.MsgInspect, ext)
	}
	// A confident shared-database detection decides directly: a different
	// allowlisted format is a mismatch, an unknown one (heic, svg, …) can
	// never satisfy the claimed extension.
	if detected := detectExtensionViaMimetype(buf); detected != "" {
		if detected != normalizedExt {
			return errors.New(errorrender.MsgValidateImageMagic)
		}
		return nil
	}
	// No confident detection (e.g. truncated header): fall back to the
	// allowlist-scoped per-format magic check.
	signatures := imageSignaturesByExtension(normalizedExt)
	for _, sig := range signatures {
		if len(buf) < sig.offset+len(sig.bytes) {
			return errors.New(errorrender.MsgInspectDetectedBytes)
		}
		for i, b := range sig.bytes {
			if buf[sig.offset+i] != b {
				return errors.New(errorrender.MsgValidateImageMagic)
			}
		}
	}
	return nil
}
