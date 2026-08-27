package imageformat

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"io"
)

func init() {
	image.RegisterFormat("avif", "????ftyp", nil, decodeAVIFConfig)
}

func decodeAVIFConfig(r io.Reader) (image.Config, error) {
	header := make([]byte, 64*1024)
	n, err := io.ReadFull(r, header)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return image.Config{}, err
	}
	header = header[:n]
	return parseAVIFConfig(header)
}

func parseAVIFConfig(data []byte) (image.Config, error) {
	ispeTag := []byte("ispe")
	idx := 0
	for {
		pos := bytes.Index(data[idx:], ispeTag)
		if pos < 0 {
			return image.Config{}, errors.New("avif: ispe box not found")
		}
		absPos := idx + pos
		if absPos >= 4 && absPos+16 <= len(data) {
			boxSize := binary.BigEndian.Uint32(data[absPos-4 : absPos])
			if boxSize == 20 || boxSize == 0 {
				width := binary.BigEndian.Uint32(data[absPos+8 : absPos+12])
				height := binary.BigEndian.Uint32(data[absPos+12 : absPos+16])
				if width > 0 && height > 0 {
					return image.Config{
						Width:  int(width),
						Height: int(height),
					}, nil
				}
			}
		}
		idx = absPos + 4
		if idx >= len(data) {
			break
		}
	}
	return image.Config{}, errors.New("avif: valid dimensions not found in ispe box")
}
