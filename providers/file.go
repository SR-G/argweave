package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// DefaultSearchDirs returns the directories checked by DefaultConfigFile
// when AppConfig.ConfigSearchPaths is left empty: the current working
// directory, then the directory containing the running binary (if it can
// be resolved and differs from the working directory).
func DefaultSearchDirs() []string {
	dirs := []string{"."}
	exe, err := os.Executable()
	if err != nil {
		return dirs
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if dir := filepath.Dir(exe); dir != "." {
		dirs = append(dirs, dir)
	}
	return dirs
}

// DefaultConfigFile auto-detects a config file based on the running
// binary's name: "<name>.json", then "<name>.toml", checked in each of
// dirs in order. Returns "" if none exist.
func DefaultConfigFile(dirs []string) string {
	if len(os.Args) == 0 || os.Args[0] == "" {
		return ""
	}
	base := filepath.Base(os.Args[0])
	base = strings.TrimSuffix(base, filepath.Ext(base))

	for _, dir := range dirs {
		for _, ext := range []string{".json", ".toml"} {
			candidate := filepath.Join(dir, base+ext)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	return ""
}

// LoadConfigFile reads and decodes path (JSON or TOML, selected by its
// file extension) into a generic string-keyed map. JSON numbers are kept
// as json.Number to avoid float64 precision loss for large integers.
func LoadConfigFile(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	m := map[string]interface{}{}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&m); err != nil {
			return nil, fmt.Errorf("parsing JSON config file %s: %w", path, err)
		}
	case ".toml":
		if err := toml.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parsing TOML config file %s: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("unsupported config file extension %q for %s (expected .json or .toml)", filepath.Ext(path), path)
	}
	return m, nil
}

// ConfigValueString converts a value decoded from JSON/TOML into the
// plain string representation used throughout the rest of the value
// resolution pipeline (mirroring CLI flags and environment variables,
// both of which are also plain strings).
func ConfigValueString(v interface{}, isSlice bool) string {
	if isSlice {
		if arr, ok := v.([]interface{}); ok {
			parts := make([]string, len(arr))
			for i, item := range arr {
				parts[i] = scalarString(item)
			}
			return strings.Join(parts, ",")
		}
	}
	return scalarString(v)
}

// scalarString formats a single decoded value, special-casing
// json.Number so integers round-trip exactly instead of going through a
// lossy float64 conversion.
func scalarString(v interface{}) string {
	if n, ok := v.(json.Number); ok {
		return n.String()
	}
	return fmt.Sprint(v)
}
