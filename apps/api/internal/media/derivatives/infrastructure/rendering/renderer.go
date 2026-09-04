package rendering

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"ideaboard/api/internal/media/derivatives/domain"
	"ideaboard/api/internal/media/derivatives/ports"
	"ideaboard/api/internal/media/imageformat"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

type GoRenderer struct{}

func (GoRenderer) Render(_ context.Context, data []byte, _ string, maxSide int) ([]byte, int, int, string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, "", fmt.Errorf("decode source image: %w", err)
	}
	bounds := img.Bounds()
	longest := max(bounds.Dx(), bounds.Dy())
	width, height := bounds.Dx(), bounds.Dy()
	if longest > maxSide {
		scale := float64(maxSide) / float64(longest)
		width = max(1, int(float64(width)*scale+0.5))
		height = max(1, int(float64(height)*scale+0.5))
	}
	resized := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(resized, resized.Bounds(), img, bounds, draw.Over, nil)
	transparent := hasTransparency(resized)
	var output bytes.Buffer
	if transparent {
		if err := png.Encode(&output, resized); err != nil {
			return nil, 0, 0, "", err
		}
		return output.Bytes(), width, height, domain.FormatPNG, nil
	}
	if err := jpeg.Encode(&output, resized, &jpeg.Options{Quality: 82}); err != nil {
		return nil, 0, 0, "", err
	}
	return output.Bytes(), width, height, domain.FormatJPEG, nil
}

type VipsRenderer struct {
	binary string
}

func NewRenderer() ports.Renderer {
	binary := strings.TrimSpace(os.Getenv("IDEABOARD_VIPS_BINARY"))
	if binary == "" {
		return GoRenderer{}
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return GoRenderer{}
	}
	return VipsRenderer{binary: resolved}
}

func (r VipsRenderer) Render(ctx context.Context, data []byte, sourceName string, maxSide int) ([]byte, int, int, string, error) {
	if strings.TrimSpace(r.binary) == "" {
		return nil, 0, 0, "", fmt.Errorf("libvips binary is not configured")
	}
	ext := filepath.Ext(sourceName)
	if ext == "" {
		ext = ".source"
	}
	input, err := os.CreateTemp("", "ideaboard-vips-*"+ext)
	if err != nil {
		return nil, 0, 0, "", fmt.Errorf("create libvips input: %w", err)
	}
	inputPath := input.Name()
	defer os.Remove(inputPath)
	defer os.Remove(inputPath + ".webp")
	if err := input.Chmod(0600); err != nil {
		_ = input.Close()
		return nil, 0, 0, "", err
	}
	if _, err := input.Write(data); err != nil {
		_ = input.Close()
		return nil, 0, 0, "", fmt.Errorf("write libvips input: %w", err)
	}
	if err := input.Close(); err != nil {
		return nil, 0, 0, "", err
	}
	outputPath := inputPath + ".webp"
	cmd := exec.CommandContext(ctx, r.binary, "thumbnail", inputPath, outputPath, strconv.Itoa(maxSide), "--size", "both")
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, 0, 0, "", fmt.Errorf("libvips thumbnail failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	encoded, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, 0, 0, "", fmt.Errorf("read libvips output: %w", err)
	}
	facts, err := imageformat.InspectBytes(encoded, ".webp")
	if err != nil {
		return nil, 0, 0, "", fmt.Errorf("validate libvips output: %w", err)
	}
	return encoded, facts.Width, facts.Height, domain.FormatWebP, nil
}

func hasTransparency(img image.Image) bool {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA).A < 255 {
				return true
			}
		}
	}
	return false
}
