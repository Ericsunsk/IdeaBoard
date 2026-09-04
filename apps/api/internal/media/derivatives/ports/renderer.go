package ports

import "context"

type Renderer interface {
	Render(ctx context.Context, data []byte, sourceName string, maxSide int) ([]byte, int, int, string, error)
}
