package argweave

import (
	"fmt"
	"regexp"
	"strings"
)

// FieldSpec is the parsed representation of an `arg:"..."` struct tag.
//
// The tag syntax is a comma separated list of `key` or `key=value` tokens,
// heavily inspired by the Rust "clap" crate's `#[arg(...)]` attribute:
//
//	`arg:"short,long,env=PORT,default=8080,help=Port to listen on"`
//	`arg:"short=p,long=port,required,help=Port to listen on"`
//
// Recognized keys:
//
//	short           - use the first letter of the field name as short flag
//	short=X         - use X (a single character) as the short flag
//	long            - derive a kebab-case long flag from the field name
//	long=name       - use "name" as the long flag
//	alias=name      - additional long name(s) for the flag; may repeat
//	env=NAME        - fall back to the environment variable NAME
//	env_allow_empty - treat an explicitly empty env=NAME variable as set,
//	                  instead of falling through to the next provider
//	default=value   - default value used when not set on the CLI or in env
//	required        - fail parsing if no value can be resolved
//	positional      - bind the value from a positional argument instead of a flag
//	hidden          - omit the option from --help and generated docs
//	value_name=NAME - override the placeholder shown in --help (e.g. <NAME>)
//	group=NAME      - group the option under a named help section
//	requires=FIELD  - require another field when this option is resolved; may repeat
//	conflicts=FIELD - reject this option when another field is resolved; may repeat
//	secret          - redact the value from ConfigJSON output
//	file            - treat the resolved value as a file path and read its contents
//	help=text       - description shown in --help and in generated docs
//	description=... - alias for help
//
// A comma that is part of a value (e.g. inside `help=...`) must be escaped
// with a backslash: `\,`.
type FieldSpec struct {
	Short                      string
	Long                       string
	Aliases                    []string
	Env                        string
	EnvAllowEmpty              bool
	Default                    string
	HasDefault                 bool
	Required                   bool
	Positional                 bool
	Hidden                     bool
	ValueNameInHelpDescription string
	Group                      string
	Requires                   []string
	Conflicts                  []string
	Secret                     bool
	FromFile                   bool
	Help                       string
}

// ParseTag parses the content of an `arg` struct tag for a field named
// fieldName (used to derive short/long flags when no explicit value is
// given).
func ParseTag(tag string, fieldName string) (FieldSpec, error) {
	var spec FieldSpec

	for _, token := range splitTag(tag) {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		key, value, hasValue := token, "", false
		if idx := strings.Index(token, "="); idx >= 0 {
			key = token[:idx]
			value = unescapeTagValue(token[idx+1:])
			hasValue = true
		}
		key = strings.TrimSpace(key)

		switch key {
		case "short":
			if hasValue {
				if len(value) != 1 {
					return spec, fmt.Errorf("argweave: short flag for field %q must be a single character, got %q", fieldName, value)
				}
				spec.Short = value
			} else {
				spec.Short = strings.ToLower(fieldName[:1])
			}
		case "long":
			if hasValue {
				spec.Long = value
			} else {
				spec.Long = camelToKebab(fieldName)
			}
		case "alias":
			if !hasValue {
				return spec, fmt.Errorf("argweave: alias key for field %q requires a value", fieldName)
			}
			spec.Aliases = append(spec.Aliases, value)
		case "env":
			if !hasValue {
				return spec, fmt.Errorf("argweave: env key for field %q requires a value", fieldName)
			}
			spec.Env = value
		case "env_allow_empty":
			if hasValue {
				spec.EnvAllowEmpty = value == "true"
			} else {
				spec.EnvAllowEmpty = true
			}
		case "default":
			spec.Default = value
			spec.HasDefault = true
		case "required":
			if hasValue {
				spec.Required = value == "true"
			} else {
				spec.Required = true
			}
		case "positional":
			if hasValue {
				spec.Positional = value == "true"
			} else {
				spec.Positional = true
			}
		case "hidden":
			if hasValue {
				spec.Hidden = value == "true"
			} else {
				spec.Hidden = true
			}
		case "value_name":
			if !hasValue {
				return spec, fmt.Errorf("argweave: value_name key for field %q requires a value", fieldName)
			}
			spec.ValueNameInHelpDescription = value
		case "group":
			if !hasValue {
				return spec, fmt.Errorf("argweave: group key for field %q requires a value", fieldName)
			}
			spec.Group = value
		case "requires":
			if !hasValue {
				return spec, fmt.Errorf("argweave: requires key for field %q requires a value", fieldName)
			}
			spec.Requires = append(spec.Requires, value)
		case "conflicts":
			if !hasValue {
				return spec, fmt.Errorf("argweave: conflicts key for field %q requires a value", fieldName)
			}
			spec.Conflicts = append(spec.Conflicts, value)
		case "secret":
			if hasValue {
				spec.Secret = value == "true"
			} else {
				spec.Secret = true
			}
		case "file", "from_file":
			if hasValue {
				spec.FromFile = value == "true"
			} else {
				spec.FromFile = true
			}
		case "help", "description":
			spec.Help = value
		default:
			return spec, fmt.Errorf("argweave: unknown tag key %q on field %q", key, fieldName)
		}
	}

	if spec.Positional && (spec.Short != "" || spec.Long != "") {
		return spec, fmt.Errorf("argweave: field %q cannot be positional and also define short/long flags", fieldName)
	}
	if spec.Positional && len(spec.Aliases) > 0 {
		return spec, fmt.Errorf("argweave: field %q cannot be positional and also define aliases", fieldName)
	}

	return spec, nil
}

// splitTag splits a tag string on unescaped commas.
func splitTag(tag string) []string {
	var (
		tokens []string
		cur    strings.Builder
		escape bool
	)
	for _, r := range tag {
		switch {
		case escape:
			cur.WriteRune('\\')
			cur.WriteRune(r)
			escape = false
		case r == '\\':
			escape = true
		case r == ',':
			tokens = append(tokens, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if escape {
		cur.WriteRune('\\')
	}
	tokens = append(tokens, cur.String())
	return tokens
}

func unescapeTagValue(s string) string {
	return strings.ReplaceAll(s, `\,`, ",")
}

var (
	kebabAcronymRe = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
	kebabWordRe    = regexp.MustCompile(`([a-z0-9])([A-Z])`)
)

// camelToKebab converts a Go identifier (e.g. "APIKey", "MaxRetries") into a
// kebab-case flag name (e.g. "api-key", "max-retries").
func camelToKebab(s string) string {
	s = kebabAcronymRe.ReplaceAllString(s, "$1-$2")
	s = kebabWordRe.ReplaceAllString(s, "$1-$2")
	return strings.ToLower(s)
}
