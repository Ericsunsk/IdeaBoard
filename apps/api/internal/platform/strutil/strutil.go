package strutil

// Deref returns the dereferenced string value or an empty string if ptr is nil.
func Deref(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

// Ptr returns a pointer to s, or nil if s is empty after trimming.
// Callers that need a pointer to an untrimmed value should construct it manually.
func Ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
