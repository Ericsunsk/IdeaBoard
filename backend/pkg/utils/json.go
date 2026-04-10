package utils

import "encoding/json"

func JSONMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func JSONUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
