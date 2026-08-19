package ptr

// To returns a pointer to v.
func To[T any](v T) *T {
	return &v
}

// Deref returns *p or fallback when p is nil.
func Deref[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}
