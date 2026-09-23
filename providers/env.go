package providers

import "os"

// LookupEnv resolves name (optionally prefixed) from the environment. An
// unset variable is always treated as "not provided". An explicitly empty
// variable is also treated as "not provided", matching the behavior of the
// other providers, unless allowEmpty is true (set via the field's
// env_allow_empty tag).
func LookupEnv(name, prefix string, allowEmpty bool) (string, bool) {
	if name == "" {
		return "", false
	}
	v, ok := os.LookupEnv(prefix + name)
	if !ok || (v == "" && !allowEmpty) {
		return "", false
	}
	return v, true
}
