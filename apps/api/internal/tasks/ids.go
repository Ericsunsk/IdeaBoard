package tasks

func IDPtr(id int64) *int64 {
	v := id
	return &v
}

func StringPtr(s string) *string {
	v := s
	return &v
}

func IDValue(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}
