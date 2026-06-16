package tasks

func IDPtr(id int64) *int64 {
	v := id
	return &v
}

func StringPtr(s string) *string {
	v := s
	return &v
}

func Int64Ptr(v int64) *int64 {
	ptr := v
	return &ptr
}

func IDValue(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}
