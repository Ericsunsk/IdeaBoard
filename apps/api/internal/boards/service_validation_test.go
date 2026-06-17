package boards

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeBoardID(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "valid generated id", raw: " board_1712345678_ab12cd ", want: "board_1712345678_ab12cd"},
		{name: "reject slash", raw: "board/../../x", want: ""},
		{name: "reject over max length", raw: strings.Repeat("a", maxBoardIDLen+1), want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeBoardID(tt.raw); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMarshalStateLimits(t *testing.T) {
	t.Run("valid envelope", func(t *testing.T) {
		raw, version, err := marshalState(validBoardState(1, 100))
		if err != nil {
			t.Fatalf("marshalState returned error: %v", err)
		}
		if raw == "" {
			t.Fatal("raw state should not be empty")
		}
		if version != 1 {
			t.Fatalf("schema version = %d, want 1", version)
		}
	})

	t.Run("missing schema version", func(t *testing.T) {
		_, _, err := marshalState(map[string]interface{}{
			"imageNodes":  []interface{}{},
			"promptNodes": []interface{}{},
			"fusionNodes": []interface{}{},
			"connections": []interface{}{},
			"viewport": map[string]interface{}{
				"canvasX":   0,
				"canvasY":   0,
				"zoomLevel": 100,
			},
			"updatedAt": 1,
		})
		if !errors.Is(err, ErrInvalidBoardInput) {
			t.Fatalf("err = %v, want ErrInvalidBoardInput", err)
		}
	})

	t.Run("state byte limit", func(t *testing.T) {
		_, _, err := marshalState(map[string]interface{}{
			"payload": strings.Repeat("x", maxBoardStateBytes+1),
		})
		if !errors.Is(err, ErrInvalidBoardInput) {
			t.Fatalf("err = %v, want ErrInvalidBoardInput", err)
		}
	})

	t.Run("state depth limit", func(t *testing.T) {
		var current interface{} = "leaf"
		for i := 0; i < maxBoardStateDepth+1; i++ {
			current = map[string]interface{}{"next": current}
		}
		_, _, err := marshalState(map[string]interface{}{"root": current})
		if !errors.Is(err, ErrInvalidBoardInput) {
			t.Fatalf("err = %v, want ErrInvalidBoardInput", err)
		}
	})
}

func validBoardState(schemaVersion int, zoomLevel int) map[string]interface{} {
	return map[string]interface{}{
		"schemaVersion": schemaVersion,
		"imageNodes":    []interface{}{},
		"promptNodes":   []interface{}{},
		"fusionNodes":   []interface{}{},
		"connections":   []interface{}{},
		"viewport": map[string]interface{}{
			"canvasX":   0,
			"canvasY":   0,
			"zoomLevel": zoomLevel,
		},
		"updatedAt": 1,
	}
}
