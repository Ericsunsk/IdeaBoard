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
		raw, version, err := marshalState(validBoardState(CurrentBoardStateSchemaVersion, 100))
		if err != nil {
			t.Fatalf("marshalState returned error: %v", err)
		}
		if raw == "" {
			t.Fatal("raw state should not be empty")
		}
		if version != CurrentBoardStateSchemaVersion {
			t.Fatalf("schema version = %d, want %d", version, CurrentBoardStateSchemaVersion)
		}
	})

	t.Run("valid v3 group", func(t *testing.T) {
		state := validBoardState(CurrentBoardStateSchemaVersion, 100)
		state["groups"] = []interface{}{
			validBoardStateGroup(map[string]interface{}{
				"nodeIds": []interface{}{"image-1", "prompt-1000.25"},
			}),
		}
		if _, _, err := marshalState(state); err != nil {
			t.Fatalf("marshalState returned error: %v", err)
		}
	})

	t.Run("missing schema version", func(t *testing.T) {
		_, _, err := marshalState(map[string]interface{}{
			"imageNodes":  []interface{}{},
			"promptNodes": []interface{}{},
			"fusionNodes": []interface{}{},
			"connections": []interface{}{},
			"updatedAt":   1,
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("reject old schema version", func(t *testing.T) {
		state := validBoardState(CurrentBoardStateSchemaVersion-1, 100)
		_, _, err := marshalState(state)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("reject unknown top-level field", func(t *testing.T) {
		state := validBoardState(CurrentBoardStateSchemaVersion, 100)
		state["unexpected"] = true
		_, _, err := marshalState(state)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("reject viewport", func(t *testing.T) {
		state := validBoardState(CurrentBoardStateSchemaVersion, 100)
		state["viewport"] = map[string]interface{}{"x": 1, "y": 2, "zoom": 1}
		_, _, err := marshalState(state)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("valid optional envelope metadata", func(t *testing.T) {
		state := validBoardState(CurrentBoardStateSchemaVersion, 100)
		state["mediaRefs"] = []interface{}{
			map[string]interface{}{"mediaAssetId": 12, "refType": "board_node", "refId": "image-1", "role": "source"},
		}
		state["previewSnapshotDataUrl"] = "data:image/webp;base64,preview"
		state["previewSnapshotUpdatedAt"] = 123
		state["previewSnapshotVersion"] = 2
		if _, _, err := marshalState(state); err != nil {
			t.Fatalf("marshalState returned error: %v", err)
		}
	})

	t.Run("reject invalid media ref", func(t *testing.T) {
		state := validBoardState(CurrentBoardStateSchemaVersion, 100)
		state["mediaRefs"] = []interface{}{
			map[string]interface{}{"mediaAssetId": 12, "refType": "board_node", "refId": "image-1", "unexpected": true},
		}
		_, _, err := marshalState(state)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("reject empty media ref role", func(t *testing.T) {
		state := validBoardState(CurrentBoardStateSchemaVersion, 100)
		state["mediaRefs"] = []interface{}{
			map[string]interface{}{"mediaAssetId": 12, "refType": "board_node", "refId": "image-1", "role": ""},
		}
		_, _, err := marshalState(state)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("reject task output ref in board state", func(t *testing.T) {
		state := validBoardState(CurrentBoardStateSchemaVersion, 100)
		state["mediaRefs"] = []interface{}{
			map[string]interface{}{"mediaAssetId": 12, "refType": "task_output", "refId": "output-1", "role": "image"},
		}
		_, _, err := marshalState(state)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("reject invalid preview metadata", func(t *testing.T) {
		state := validBoardState(CurrentBoardStateSchemaVersion, 100)
		state["previewSnapshotVersion"] = 1.25
		_, _, err := marshalState(state)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("reject missing group geometry", func(t *testing.T) {
		state := validBoardState(CurrentBoardStateSchemaVersion, 100)
		state["groups"] = []interface{}{
			map[string]interface{}{"id": "group-1", "nodeIds": []interface{}{"image-1", "prompt-1"}},
		}
		_, _, err := marshalState(state)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("reject invalid group geometry", func(t *testing.T) {
		state := validBoardState(CurrentBoardStateSchemaVersion, 100)
		state["groups"] = []interface{}{validBoardStateGroup(map[string]interface{}{"width": 0})}
		_, _, err := marshalState(state)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("reject old group shape", func(t *testing.T) {
		state := validBoardState(CurrentBoardStateSchemaVersion, 100)
		state["groups"] = []interface{}{
			map[string]interface{}{
				"id":      "group-1",
				"nodeIds": []interface{}{"image-1", "prompt-1"},
				"bounds":  map[string]interface{}{"x": 20, "y": 20, "width": 700, "height": 360},
			},
		}
		_, _, err := marshalState(state)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("reject non-positive preview snapshot version", func(t *testing.T) {
		state := validBoardState(CurrentBoardStateSchemaVersion, 100)
		state["previewSnapshotVersion"] = 0
		_, _, err := marshalState(state)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("state byte limit", func(t *testing.T) {
		_, _, err := marshalState(map[string]interface{}{
			"payload": strings.Repeat("x", maxBoardStateBytes+1),
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("state depth limit", func(t *testing.T) {
		var current interface{} = "leaf"
		for i := 0; i < maxBoardStateDepth+1; i++ {
			current = map[string]interface{}{"next": current}
		}
		_, _, err := marshalState(map[string]interface{}{"root": current})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})
}

func validBoardState(schemaVersion int, marker int) map[string]interface{} {
	return map[string]interface{}{
		"schemaVersion": schemaVersion,
		"imageNodes":    []interface{}{},
		"promptNodes": []interface{}{
			map[string]interface{}{
				"id":     "prompt-marker",
				"marker": marker,
			},
		},
		"fusionNodes": []interface{}{},
		"connections": []interface{}{},
		"groups":      []interface{}{},
		"updatedAt":   1,
	}
}

func validBoardStateGroup(overrides map[string]interface{}) map[string]interface{} {
	group := map[string]interface{}{
		"id":      "group-1",
		"nodeIds": []interface{}{"image-1", "prompt-1"},
		"x":       20,
		"y":       20,
		"width":   700,
		"height":  360,
	}
	for key, value := range overrides {
		group[key] = value
	}
	return group
}
