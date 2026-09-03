package derivatives

import "context"

func render(data []byte, maxSide int) ([]byte, int, int, string, error) {
	return goRenderer{}.Render(context.Background(), data, "", maxSide)
}
