package worker

import (
	"encoding/json"
	"strconv"
)

func parseSizeFromParams(params map[string]interface{}) (int, int) {
	ratio, _ := params["ratio"].(string)
	quality, _ := params["quality"].(string)

	var w, h int

	// 自定义尺寸优先
	if cw, ok := readPositiveSizeParam(params, "custom_width"); ok {
		if ch, okh := readPositiveSizeParam(params, "custom_height"); okh {
			w, h = cw, ch
			// 根据清晰度缩放
			switch quality {
			case "2k":
				w, h = w*2, h*2
			case "4k":
				w, h = w*4, h*4
			}
			return w, h
		}
	}

	switch ratio {
	case "3:2":
		w, h = 1536, 1024
	case "16:9":
		w, h = 1792, 1024
	case "1:1":
		w, h = 1024, 1024
	case "2:3":
		w, h = 1024, 1536
	case "3:4":
		w, h = 1024, 1365
	case "9:16":
		w, h = 1024, 1792
	default:
		w, h = 1024, 1024
	}

	// 根据清晰度缩放
	switch quality {
	case "2k":
		w, h = w*2, h*2
	case "4k":
		w, h = w*4, h*4
	}

	return w, h
}

func readPositiveSizeParam(params map[string]interface{}, key string) (int, bool) {
	if params == nil {
		return 0, false
	}
	switch value := params[key].(type) {
	case float64:
		if value > 0 {
			return int(value), true
		}
	case float32:
		if value > 0 {
			return int(value), true
		}
	case int:
		if value > 0 {
			return value, true
		}
	case int64:
		if value > 0 {
			return int(value), true
		}
	case int32:
		if value > 0 {
			return int(value), true
		}
	case json.Number:
		if parsed, err := strconv.Atoi(string(value)); err == nil && parsed > 0 {
			return parsed, true
		}
	}
	return 0, false
}
