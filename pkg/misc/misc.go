package misc

// StringPtr converts a string to a pointer to a string.
func StringPtr(s string) *string {
	return &s
}
