package providers

// Resolved reports whether the flags/positional-argument provider already
// produced a value for a field. Actual command line parsing stays in the
// core parser (it needs deep access to reflect-backed field metadata), so
// this provider is just a thin signal over that already-parsed state.
func Resolved(setFromCLI bool) bool {
	return setFromCLI
}
