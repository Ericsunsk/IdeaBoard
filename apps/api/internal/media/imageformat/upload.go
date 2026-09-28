package imageformat

import (
	"fmt"
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
		return Facts{}, fmt.Errorf(msgInspectDetectedBytes)
	}
	f, err := fh.Open()
	if err != nil {
		return Facts{}, fmt.Errorf(msgInspectFileHeader, err)
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
			return Facts{}, fmt.Errorf(msgInspect3, err)
		}
		if _, _, err := image.Decode(f); err != nil {
			return Facts{}, fmt.Errorf(msgInspectFileHeader2, err)
		}
	}
	return facts, nil
}

// InspectDetected detects the canonical format from the source bytes before
// running the normal signature and image-config validation.
func InspectDetected(source io.ReadSeeker) (Facts, error) {
	if source == nil {
		return Facts{}, fmt.Errorf(msgInspectDetectedBytes)
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return Facts{}, fmt.Errorf(msgInspect3, err)
	}
	header := make([]byte, 64*1024)
	n, readErr := io.ReadFull(source, header)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return Facts{}, fmt.Errorf(msgInspect3, readErr)
	}
	ext := DetectImageExtensionFromBytes(header[:n])
	if ext == "" {
		return Facts{}, fmt.Errorf(msgInspectDetectedBytes)
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return Facts{}, fmt.Errorf(msgInspect3, err)
	}
	return Inspect(source, ext)
}

// ValidateImageBytes verifies in-memory bytes match the allowed image extension.
func ValidateImageBytes(data []byte, ext string) error {
	normalizedExt, ok := NormalizeImageExtension(ext)
	if !ok {
		return fmt.Errorf(msgInspect, ext)
	}

	signatures := imageSignaturesByExtension(normalizedExt)
	required := requiredImageHeaderLength(signatures)
	if len(data) < required {
		return fmt.Errorf(msgInspectDetectedBytes)
	}

	return validateImageMagic(data[:required], normalizedExt)
}

// ValidateImageReader verifies the image signature from a stream without
// requiring callers to materialize the whole file in memory.
func ValidateImageReader(r io.Reader, ext string) error {
	normalizedExt, ok := NormalizeImageExtension(ext)
	if !ok {
		return fmt.Errorf(msgInspect, ext)
	}
	if r == nil {
		return fmt.Errorf(msgInspectDetectedBytes)
	}

	signatures := imageSignaturesByExtension(normalizedExt)
	required := requiredImageHeaderLength(signatures)
	buf := make([]byte, required)
	n, err := io.ReadFull(r, buf)
	if err != nil || n < required {
		return fmt.Errorf(msgInspectDetectedBytes)
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
		return fmt.Errorf(msgInspect, ext)
	}
	// A confident shared-database detection decides directly: a different
	// allowlisted format is a mismatch, an unknown one (heic, svg, …) can
	// never satisfy the claimed extension.
	if detected := detectExtensionViaMimetype(buf); detected != "" {
		if detected != normalizedExt {
			return fmt.Errorf(msgValidateImageMagic)
		}
		return nil
	}
	// No confident detection (e.g. truncated header): fall back to the
	// allowlist-scoped per-format magic check.
	signatures := imageSignaturesByExtension(normalizedExt)
	for _, sig := range signatures {
		if len(buf) < sig.offset+len(sig.bytes) {
			return fmt.Errorf(msgInspectDetectedBytes)
		}
		for i, b := range sig.bytes {
			if buf[sig.offset+i] != b {
				return fmt.Errorf(msgValidateImageMagic)
			}
		}
	}
	return nil
}
