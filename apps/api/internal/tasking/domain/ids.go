package domain

func StringPtr(s string) *string {
	v := s
	return &v
}

func Int64Ptr(value int64) *int64 {
	return &value
}

func IDValue(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}
