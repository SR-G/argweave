package providers

// Default resolves a field's default=value tag: value is only usable when
// hasDefault is true (a field with no default tag never resolves here).
func Default(value string, hasDefault bool) (string, bool) {
	return value, hasDefault
}
