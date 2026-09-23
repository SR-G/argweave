package argweave

// TAG_KEY is the struct tag key recognized by this library, e.g.
// `arg:"long=port"`. Exposed so tools like cmd/weavedoc can look up the
// same tag without hardcoding the string.
const TAG_KEY = "arg"

// REDACTED_PLACEHOLDER replaces a `secret`-tagged field's value wherever
// resolved configuration is rendered (ConfigJSON, --help, and weavedoc's
// generated Markdown/JSON Schema output).
const REDACTED_PLACEHOLDER = "[REDACTED]"

const (
	// OS_EXIT_OK is the conventional success exit code.
	OS_EXIT_OK = 0
	// OS_EXIT_INTERNAL_ERROR indicates an internal output/rendering failure.
	OS_EXIT_INTERNAL_ERROR = 1
	// OS_EXIT_OPTIONS_IN_ERROR indicates invalid command-line input.
	OS_EXIT_OPTIONS_IN_ERROR = 2
)

const (
	// EXIT_CODE_PURPOSE_NORMAL_EXIT names the success exit code.
	EXIT_CODE_PURPOSE_NORMAL_EXIT = "normal_exit"
	// EXIT_CODE_PURPOSE_INTERNAL_ERROR names the internal-error exit code.
	EXIT_CODE_PURPOSE_INTERNAL_ERROR = "internal_error"
	// EXIT_CODE_PURPOSE_OPTIONS_IN_ERROR names the invalid-options exit code.
	EXIT_CODE_PURPOSE_OPTIONS_IN_ERROR = "options_in_error"
)
