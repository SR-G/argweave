// Package argweave provides a small library to define CLI
// flags and environment variable bindings via Go struct tags, inspired by
// the Rust "clap" crate.
package argweave

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"forgejo.tensin.org/SR-G/argweave/providers"
)

// Command identifies a built-in parser action requested by the arguments.
type Command int

const (
	// COMMAND_NONE indicates that normal parsing should continue.
	COMMAND_NONE Command = iota
	// COMMAND_HELP requests rendering the generated help text.
	COMMAND_HELP
	// COMMAND_VERSION requests rendering the program version.
	COMMAND_VERSION
	// COMMAND_PRINT_CONFIG requests rendering the resolved configuration.
	COMMAND_PRINT_CONFIG
)

// String implements fmt.Stringer, so a Command prints as its symbolic name
// (e.g. in log statements) instead of a bare integer.
func (c Command) String() string {
	switch c {
	case COMMAND_NONE:
		return "COMMAND_NONE"
	case COMMAND_HELP:
		return "COMMAND_HELP"
	case COMMAND_VERSION:
		return "COMMAND_VERSION"
	case COMMAND_PRINT_CONFIG:
		return "COMMAND_PRINT_CONFIG"
	default:
		return fmt.Sprintf("Command(%d)", int(c))
	}
}

// Value is implemented by types that need custom parsing from a single
// string, mirroring the standard library's flag.Value interface. If the
// address of a tagged field implements Value, it is used instead of the
// built-in type conversion.
type Value interface {
	String() string
	Set(string) error
}

// Validator is implemented by configuration structs that need cross-field
// validation after all providers have been resolved. All returned non-nil
// errors are reported together.
type Validator interface {
	Validate() []error
}

// ProviderKind identifies a source of configuration values.
type ProviderKind string

// SourceInfo describes how a configuration field was resolved on the last
// Parse call.
type SourceInfo struct {
	Provider ProviderKind
	Key      string
	Detail   string
	Redacted bool
}

const (
	// ProviderEnv resolves values from environment variables (via a
	// field's env=NAME tag, optionally prefixed by AppConfig.EnvPrefix).
	ProviderEnv ProviderKind = "env"
	// ProviderFlags resolves values parsed from the command line (flags
	// and positional arguments).
	ProviderFlags ProviderKind = "flags"
	// ProviderFile resolves values from a JSON or TOML configuration
	// file, auto-detected from the binary name (or overridden with
	// -c/--config when this provider is enabled).
	ProviderFile ProviderKind = "file"
	// ProviderDefault resolves a field's default=value tag.
	ProviderDefault ProviderKind = "default"
)

// DefaultProviders returns the built-in provider order used when
// AppConfig.Providers is left empty: environment variables first, then
// command line flags, then a config file, then default values. Assign a
// reordered or trimmed slice to AppConfig.Providers to change precedence
// or disable a source entirely (e.g. drop ProviderEnv to ignore
// environment variables).
func DefaultProviders() []ProviderKind {
	return []ProviderKind{ProviderEnv, ProviderFlags, ProviderFile, ProviderDefault}
}

// DefaultExitCodes returns the standard exit-code mapping used by Parser.Handle.
func DefaultExitCodes() *ExitCodesRepository {
	ecr := &ExitCodesRepository{}
	ecr.Register(OS_EXIT_OK, EXIT_CODE_PURPOSE_NORMAL_EXIT)
	ecr.Register(OS_EXIT_INTERNAL_ERROR, EXIT_CODE_PURPOSE_INTERNAL_ERROR)
	ecr.Register(OS_EXIT_OPTIONS_IN_ERROR, EXIT_CODE_PURPOSE_OPTIONS_IN_ERROR)
	return ecr
}

func validateProviders(providers []ProviderKind) error {
	seen := map[ProviderKind]bool{}
	for _, kind := range providers {
		switch kind {
		case ProviderEnv, ProviderFlags, ProviderFile, ProviderDefault:
		default:
			return fmt.Errorf("argweave: unknown provider %q", kind)
		}
		if seen[kind] {
			return fmt.Errorf("argweave: duplicate provider %q in AppConfig.Providers", kind)
		}
		seen[kind] = true
	}
	return nil
}

var (
	durationType        = reflect.TypeOf(time.Duration(0))
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	valueType           = reflect.TypeOf((*Value)(nil)).Elem()
)

// AppConfig holds the global, program-wide configuration used to render
// the --help and --version output.
type AppConfig struct {
	// Name of the program, shown in the help header and usage line.
	Name string
	// Version of the program, shown in the help header and by --version.
	Version string
	// Description of the program, shown below the header in --help.
	Description string
	// EnvPrefix, if set, is prepended to every field's env=NAME lookup.
	EnvPrefix string
	// DisableHelp disables the built-in -h/--help flag.
	DisableHelp bool
	// DisableVersion disables the built-in -V/--version flag.
	DisableVersion bool
	// Providers configures which value sources are consulted, and in
	// which order (the first provider to yield a value for a field
	// wins). Defaults to DefaultProviders() when left empty.
	Providers []ProviderKind
	// ConfigSearchPaths overrides the directories scanned for the
	// auto-detected config file (see ProviderFile). Defaults to
	// providers.DefaultSearchDirs() (the working directory, then the
	// running binary's directory) when left empty.
	ConfigSearchPaths []string
	// StrictConfig rejects unknown keys in JSON or TOML config files when
	// ProviderFile is enabled. When false, unknown keys are ignored.
	StrictConfig bool
	// HelpRenderer renders the --help page. Defaults to
	// DefaultHelpRenderer() when left nil; assign a custom HelpRenderer to
	// replace the built-in layout.
	HelpRenderer HelpRenderer
	// Validate is an optional callback for validating a configuration type
	// that cannot implement Validator directly. All returned non-nil errors
	// are reported together after parsing.
	Validate func(config interface{}) []error

	// ExitCodes controls the codes returned by Parser.ParseAndHandleExitIfNeeded(). If nil, the
	// mapping from DefaultExitCodes is used.
	ExitCodes *ExitCodesRepository
}

// entry is the internal, reflection-backed representation of one struct
// field annotated with an `arg` tag.
type entry struct {
	FieldSpec
	fieldName string
	path      []int // reflect.Value.FieldByIndex path, supports flattened/embedded structs
	kind      reflect.Kind
	isBool    bool
	isSlice   bool
	isCustom  bool // implements Value/TextUnmarshaler, or is time.Duration
}

func (e *entry) value(target reflect.Value) reflect.Value {
	v := target
	for i, idx := range e.path {
		v = v.Field(idx)
		if i < len(e.path)-1 && v.Kind() == reflect.Ptr {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
	}
	return v
}

// longTarget is what a long flag name resolves to: either a user-defined
// entry (optionally negated, for auto-generated --no-x forms) or a
// built-in action.
type longTarget struct {
	entry   *entry
	negate  bool
	builtin string // "", "help", "version", "print-config" or "config"
}

// Parser binds a struct's `arg` tagged fields to command line flags and
// environment variables.
type Parser struct {
	app     AppConfig
	target  reflect.Value // addressable struct value
	entries []*entry
	posArgs []*entry // entries with Positional set, in declaration order

	longTable  map[string]longTarget
	shortTable map[string]*entry
	providers  []ProviderKind

	helpShortTaken      bool
	versionShortTaken   bool
	configShortTaken    bool
	fileProviderEnabled bool

	resolvedFrom  map[*entry]ProviderKind
	resolvedInfo  map[*entry]SourceInfo
	exitCodes     *ExitCodesRepository
	parseDuration time.Duration
	mu            sync.Mutex
}

// New builds a Parser bound to target, which must be a non-nil pointer to
// a struct. Fields without an `arg` tag are ignored, unless they are
// struct-typed (or pointer-to-struct-typed) themselves, in which case
// their own fields are flattened into the parent automatically — this is
// how nested config groups (e.g. a DatabaseConfig field inside a Config
// struct) work through every provider, whether embedded (anonymous) or
// referenced as a plain named field.
func New(target interface{}, app AppConfig) (*Parser, error) {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return nil, fmt.Errorf("argweave: target must be a non-nil pointer to a struct")
	}
	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return nil, fmt.Errorf("argweave: target must point to a struct, got %s", elem.Kind())
	}

	providers := app.Providers
	if len(providers) == 0 {
		providers = DefaultProviders()
	}
	if err := validateProviders(providers); err != nil {
		return nil, err
	}

	ecr := app.ExitCodes
	if ecr == nil {
		ecr = DefaultExitCodes()
	} else {
		ecr.fillMissingDefaults()
	}
	if app.HelpRenderer == nil {
		app.HelpRenderer = DefaultHelpRenderer()
	}

	p := &Parser{app: app, target: elem, providers: providers, exitCodes: ecr}
	for _, kind := range providers {
		if kind == ProviderFile {
			p.fileProviderEnabled = true
		}
	}

	usedShort := map[string]string{}
	usedLong := map[string]string{}
	if err := p.collectFields(elem.Type(), nil, usedShort, usedLong, map[reflect.Type]bool{}); err != nil {
		return nil, err
	}

	for _, e := range p.entries {
		if e.Positional {
			p.posArgs = append(p.posArgs, e)
		}
	}
	for i, e := range p.posArgs {
		if e.isSlice && i != len(p.posArgs)-1 {
			return nil, fmt.Errorf("argweave: only the last positional field may be a slice (field %q)", e.fieldName)
		}
	}
	if err := p.validateFieldReferences(); err != nil {
		return nil, err
	}

	if err := p.buildTables(usedShort, usedLong); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Parser) validateFieldReferences() error {
	known := make(map[string]*entry)
	for _, e := range p.entries {
		known[e.fieldName] = e
		if e.Long != "" {
			known[e.Long] = e
		}
		for _, alias := range e.Aliases {
			known[alias] = e
		}
	}
	for _, e := range p.entries {
		for _, name := range append(append([]string{}, e.Requires...), e.Conflicts...) {
			other, ok := known[name]
			if !ok {
				return fmt.Errorf("argweave: field %s references unknown field %q", e.flagLabel(), name)
			}
			if other == e {
				return fmt.Errorf("argweave: field %s cannot require or conflict with itself", e.flagLabel())
			}
		}
	}
	return nil
}

func (p *Parser) collectFields(t reflect.Type, prefix []int, usedShort, usedLong map[string]string, active map[reflect.Type]bool) error {
	if active[t] {
		return fmt.Errorf("argweave: recursive struct flattening detected for type %s", t)
	}
	active[t] = true
	defer delete(active, t)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		path := append(append([]int{}, prefix...), i)

		tag, ok := field.Tag.Lookup(TAG_KEY)
		if !ok {
			ft := field.Type
			if ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			// Anonymous (embedded) struct fields are always flattened,
			// even when their type name is unexported (a common,
			// package-private pattern). Named struct fields are only
			// flattened when exported, since reflect cannot read/set an
			// unexported named field's contents.
			if ft.Kind() == reflect.Struct && (field.Anonymous || field.IsExported()) {
				if err := p.collectFields(ft, path, usedShort, usedLong, active); err != nil {
					return err
				}
			}
			continue
		}
		if !field.IsExported() {
			return fmt.Errorf("argweave: field %q has an arg tag but is not exported", field.Name)
		}

		spec, err := ParseTag(tag, field.Name)
		if err != nil {
			return err
		}

		kind := field.Type.Kind()
		isSlice := kind == reflect.Slice
		valueTypeForField := field.Type
		if isSlice {
			valueTypeForField = field.Type.Elem()
		}
		isCustom := isCustomType(valueTypeForField)

		if isSlice && !isSupportedScalar(valueTypeForField) && !isCustom {
			return fmt.Errorf("argweave: field %q has unsupported slice element type %s", field.Name, valueTypeForField)
		}
		if !isSlice && !isCustom {
			if !isSupportedScalar(field.Type) {
				return fmt.Errorf("argweave: field %q has unsupported type %s", field.Name, field.Type)
			}
		}

		if !spec.Positional {
			if spec.Short == "" && spec.Long == "" {
				return fmt.Errorf("argweave: field %q must define at least a short or long flag", field.Name)
			}
			if spec.Short != "" {
				if other, dup := usedShort[spec.Short]; dup {
					return fmt.Errorf("argweave: short flag -%s used by both %q and %q", spec.Short, other, field.Name)
				}
				usedShort[spec.Short] = field.Name
			}
			if spec.Long != "" {
				if other, dup := usedLong[spec.Long]; dup {
					return fmt.Errorf("argweave: long flag --%s used by both %q and %q", spec.Long, other, field.Name)
				}
				usedLong[spec.Long] = field.Name
			}
		}

		p.entries = append(p.entries, &entry{
			FieldSpec: spec,
			fieldName: field.Name,
			path:      path,
			kind:      kind,
			isBool:    kind == reflect.Bool,
			isSlice:   isSlice,
			isCustom:  isCustom,
		})
	}
	return nil
}

// buildTables constructs the long/short flag lookup tables, including
// aliases, auto-generated --no-x negations for booleans, and the built-in
// help/version/print-config actions.
func (p *Parser) buildTables(usedShort, usedLong map[string]string) error {
	p.longTable = map[string]longTarget{}
	p.shortTable = map[string]*entry{}

	addLong := func(name string, t longTarget, owner string) error {
		if existing, dup := p.longTable[name]; dup {
			return fmt.Errorf("argweave: long flag/alias --%s used by both %q and %q", name, longTargetOwner(existing), owner)
		}
		p.longTable[name] = t
		return nil
	}

	for _, e := range p.entries {
		if e.Positional {
			continue
		}
		if e.Short != "" {
			p.shortTable[e.Short] = e
		}
		if e.Long != "" {
			if err := addLong(e.Long, longTarget{entry: e}, e.fieldName); err != nil {
				return err
			}
			if e.isBool {
				if err := addLong("no-"+e.Long, longTarget{entry: e, negate: true}, e.fieldName); err != nil {
					return err
				}
			}
		}
		for _, alias := range e.Aliases {
			if err := addLong(alias, longTarget{entry: e}, e.fieldName); err != nil {
				return err
			}
			if e.isBool {
				if err := addLong("no-"+alias, longTarget{entry: e, negate: true}, e.fieldName); err != nil {
					return err
				}
			}
		}
	}

	p.helpShortTaken = usedShort["h"] != ""
	p.versionShortTaken = usedShort["V"] != ""
	p.configShortTaken = usedShort["c"] != ""

	if !p.app.DisableHelp && usedLong["help"] == "" {
		if err := addLong("help", longTarget{builtin: "help"}, "<built-in>"); err != nil {
			return err
		}
	}
	if !p.app.DisableVersion && usedLong["version"] == "" {
		if err := addLong("version", longTarget{builtin: "version"}, "<built-in>"); err != nil {
			return err
		}
	}
	if usedLong["print-config"] == "" {
		if err := addLong("print-config", longTarget{builtin: "print-config"}, "<built-in>"); err != nil {
			return err
		}
	}
	if p.fileProviderEnabled && usedLong["config"] == "" {
		if err := addLong("config", longTarget{builtin: "config"}, "<built-in>"); err != nil {
			return err
		}
	}

	return nil
}

func longTargetOwner(t longTarget) string {
	if t.entry != nil {
		return t.entry.fieldName
	}
	return "<built-in>"
}

// resolveLong finds the longTarget for name, applying clap-style
// unambiguous prefix matching when there is no exact match.
func (p *Parser) resolveLong(name string) (longTarget, error) {
	if t, ok := p.longTable[name]; ok {
		return t, nil
	}
	var matches []string
	for k := range p.longTable {
		if strings.HasPrefix(k, name) {
			matches = append(matches, k)
		}
	}
	switch len(matches) {
	case 0:
		return longTarget{}, fmt.Errorf("argweave: unknown flag --%s", name)
	case 1:
		return p.longTable[matches[0]], nil
	default:
		sort.Strings(matches)
		return longTarget{}, fmt.Errorf("argweave: ambiguous flag --%s (matches --%s)", name, strings.Join(matches, ", --"))
	}
}

// Parse parses args (typically os.Args[1:]) and populates the bound
// struct. On -h/--help it returns COMMAND_HELP. On --version/-V it
// returns COMMAND_VERSION. On --print-config it returns
// COMMAND_PRINT_CONFIG. Parse never writes anything itself; use
// GenerateHelp, Version and ConfigJSON to render the corresponding
// output. If ProviderFile is enabled, a JSON or
// TOML config file (auto-detected from the binary name, or given via
// -c/--config) is also consulted. Any other parsing problem (unknown
// flag, missing value, missing required field, invalid value) is
// returned as a plain error.
func (p *Parser) Parse(args []string) (Command, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	started := time.Now()
	defer func() {
		p.parseDuration = time.Since(started)
	}()

	values := map[*entry]string{}
	sliceAcc := map[*entry][]string{}
	setFromCLI := map[*entry]bool{}
	var positional []string
	printConfigRequested := false
	configFileOverride := ""

	rawMode := false
	i := 0
	for i < len(args) {
		a := args[i]

		switch {
		case rawMode:
			positional = append(positional, a)

		case a == "--":
			rawMode = true

		case a == "-h" && !p.helpShortTaken:
			return COMMAND_HELP, nil
		case a == "-V" && !p.versionShortTaken:
			return COMMAND_VERSION, nil

		case a == "-c" && p.fileProviderEnabled && !p.configShortTaken:
			i++
			if i >= len(args) {
				return COMMAND_NONE, fmt.Errorf("argweave: flag -c requires a value")
			}
			configFileOverride = args[i]
		case strings.HasPrefix(a, "-c=") && p.fileProviderEnabled && !p.configShortTaken:
			configFileOverride = a[len("-c="):]

		case strings.HasPrefix(a, "--"):
			name := a[2:]
			val, hasVal := "", false
			if idx := strings.Index(name, "="); idx >= 0 {
				val, hasVal = name[idx+1:], true
				name = name[:idx]
			}
			t, err := p.resolveLong(name)
			if err != nil {
				return COMMAND_NONE, err
			}
			if t.builtin != "" {
				switch t.builtin {
				case "help":
					return COMMAND_HELP, nil
				case "version":
					return COMMAND_VERSION, nil
				case "print-config":
					printConfigRequested = true
				case "config":
					if !hasVal {
						i++
						if i >= len(args) {
							return COMMAND_NONE, fmt.Errorf("argweave: flag --config requires a value")
						}
						val = args[i]
					}
					configFileOverride = val
				}
				break
			}
			e := t.entry
			if t.negate {
				if hasVal {
					return COMMAND_NONE, fmt.Errorf("argweave: negated flag --%s does not take a value", name)
				}
				values[e] = "false"
				setFromCLI[e] = true
				break
			}
			if e.isBool && !hasVal {
				values[e] = "true"
				setFromCLI[e] = true
				break
			}
			if !hasVal {
				i++
				if i >= len(args) {
					return COMMAND_NONE, fmt.Errorf("argweave: flag --%s requires a value", name)
				}
				val = args[i]
			}
			if e.isSlice {
				sliceAcc[e] = append(sliceAcc[e], val)
			} else {
				values[e] = val
			}
			setFromCLI[e] = true

		case strings.HasPrefix(a, "-") && len(a) > 1:
			name := a[1:]
			if idx := strings.Index(name, "="); idx >= 0 {
				key, val := name[:idx], name[idx+1:]
				if len(key) != 1 {
					return COMMAND_NONE, fmt.Errorf("argweave: invalid short flag %q", a)
				}
				e, ok := p.shortTable[key]
				if !ok {
					return COMMAND_NONE, fmt.Errorf("argweave: unknown flag -%s", key)
				}
				if e.isSlice {
					sliceAcc[e] = append(sliceAcc[e], val)
				} else {
					values[e] = val
				}
				setFromCLI[e] = true
				break
			}
			if len(name) == 1 {
				e, ok := p.shortTable[name]
				if !ok {
					return COMMAND_NONE, fmt.Errorf("argweave: unknown flag -%s", name)
				}
				if e.isBool {
					values[e] = "true"
					setFromCLI[e] = true
					break
				}
				i++
				if i >= len(args) {
					return COMMAND_NONE, fmt.Errorf("argweave: flag -%s requires a value", name)
				}
				if e.isSlice {
					sliceAcc[e] = append(sliceAcc[e], args[i])
				} else {
					values[e] = args[i]
				}
				setFromCLI[e] = true
				break
			}
			// combined short boolean flags, e.g. -vh (also recognizes the
			// built-in -h/-V shorts, mirroring their standalone handling above)
			for _, c := range name {
				switch {
				case c == 'h' && !p.helpShortTaken:
					return COMMAND_HELP, nil
				case c == 'V' && !p.versionShortTaken:
					return COMMAND_VERSION, nil
				}
				e, ok := p.shortTable[string(c)]
				if !ok || !e.isBool {
					return COMMAND_NONE, fmt.Errorf("argweave: unknown or non-boolean flag -%c in %q", c, a)
				}
				values[e] = "true"
				setFromCLI[e] = true
			}

		default:
			positional = append(positional, a)
		}
		i++
	}

	if err := p.assignPositional(positional, values, sliceAcc, setFromCLI); err != nil {
		return COMMAND_NONE, err
	}

	for e, vals := range sliceAcc {
		values[e] = strings.Join(vals, ",")
	}

	var fileValues map[string]interface{}
	configPath := ""
	if p.fileProviderEnabled {
		path := configFileOverride
		if path == "" {
			dirs := p.app.ConfigSearchPaths
			if len(dirs) == 0 {
				dirs = providers.DefaultSearchDirs()
			}
			path = providers.DefaultConfigFile(dirs)
		}
		if path != "" {
			configPath = path
			v, err := providers.LoadConfigFile(path)
			if err != nil {
				return COMMAND_NONE, fmt.Errorf("argweave: %w", err)
			}
			if p.app.StrictConfig {
				if err := p.validateConfigKeys(v); err != nil {
					return COMMAND_NONE, err
				}
			}
			fileValues = v
		}
	}

	var missing []string
	resolvedSet := map[*entry]bool{}
	p.resolvedFrom = map[*entry]ProviderKind{}
	p.resolvedInfo = map[*entry]SourceInfo{}
	for _, e := range p.entries {
		resolved := false
		info := SourceInfo{}
		for _, kind := range p.providers {
			switch kind {
			case ProviderFlags:
				resolved = providers.Resolved(setFromCLI[e])
				if resolved {
					info = SourceInfo{Provider: kind, Key: e.flagLabel(), Redacted: e.Secret}
				}
			case ProviderEnv:
				if v, ok := providers.LookupEnv(e.Env, p.app.EnvPrefix, e.EnvAllowEmpty); ok {
					values[e] = v
					resolved = true
					info = SourceInfo{Provider: kind, Key: p.app.EnvPrefix + e.Env, Redacted: e.Secret}
				}
			case ProviderFile:
				if fileValues != nil {
					for _, key := range e.configKeys() {
						if raw, ok := fileValues[key]; ok {
							values[e] = providers.ConfigValueString(raw, e.isSlice)
							resolved = true
							info = SourceInfo{Provider: kind, Key: key, Detail: configPath, Redacted: e.Secret}
							break
						}
					}
				}
			case ProviderDefault:
				if v, ok := providers.Default(e.Default, e.HasDefault); ok {
					values[e] = v
					resolved = true
					info = SourceInfo{Provider: kind, Key: e.configKey(), Redacted: e.Secret}
				}
			}
			if resolved {
				p.resolvedFrom[e] = kind
				p.resolvedInfo[e] = info
				break
			}
		}
		if resolved {
			resolvedSet[e] = true
		} else if e.Required {
			missing = append(missing, e.flagLabel())
		}
	}
	if len(missing) > 0 {
		return COMMAND_NONE, fmt.Errorf("argweave: missing required flag(s)/argument(s): %s", strings.Join(missing, ", "))
	}

	for _, e := range p.entries {
		if !resolvedSet[e] {
			continue
		}
		val, ok := values[e]
		if !ok {
			continue
		}
		if err := setValue(e.value(p.target), e, val); err != nil {
			source := string(p.resolvedFrom[e])
			if source == "" {
				source = "unknown source"
			}
			return COMMAND_NONE, fmt.Errorf("argweave: invalid value for %s from %s: %w", e.flagLabel(), source, err)
		}
	}

	if err := p.validateResolved(resolvedSet, values); err != nil {
		return COMMAND_NONE, err
	}

	if printConfigRequested {
		return COMMAND_PRINT_CONFIG, nil
	}

	return COMMAND_NONE, nil
}

// ParsingDuration returns the elapsed time recorded by the most recent call
// to Parse. It returns zero before Parse has been called.
func (p *Parser) ParsingDuration() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.parseDuration
}

func (p *Parser) validateConfigKeys(values map[string]interface{}) error {
	known := make(map[string]bool)
	for _, e := range p.entries {
		for _, key := range e.configKeys() {
			known[key] = true
		}
	}
	var unknown []string
	for key := range values {
		if !known[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("argweave: unknown config key(s): %s", strings.Join(unknown, ", "))
}

func (p *Parser) validateResolved(resolvedSet map[*entry]bool, values map[*entry]string) error {
	byName := make(map[string]*entry)
	var validationErrors []error
	for _, e := range p.entries {
		byName[e.fieldName] = e
		if e.Long != "" {
			byName[e.Long] = e
		}
		for _, alias := range e.Aliases {
			byName[alias] = e
		}
	}
	active := func(e *entry) bool {
		if !resolvedSet[e] {
			return false
		}
		if e.isBool {
			return values[e] == "true"
		}
		return values[e] != ""
	}
	for _, e := range p.entries {
		if !active(e) {
			continue
		}
		for _, name := range e.Requires {
			other := byName[name]
			if other == nil {
				validationErrors = append(validationErrors, fmt.Errorf("argweave: field %s requires unknown field %q", e.flagLabel(), name))
				continue
			}
			if !active(other) {
				validationErrors = append(validationErrors, fmt.Errorf("argweave: field %s requires %s", e.flagLabel(), other.flagLabel()))
			}
		}
		for _, name := range e.Conflicts {
			other := byName[name]
			if other == nil {
				validationErrors = append(validationErrors, fmt.Errorf("argweave: field %s conflicts with unknown field %q", e.flagLabel(), name))
				continue
			}
			if active(other) {
				validationErrors = append(validationErrors, fmt.Errorf("argweave: field %s conflicts with %s", e.flagLabel(), other.flagLabel()))
			}
		}
	}
	if validator, ok := p.target.Addr().Interface().(Validator); ok {
		validationErrors = appendNonNil(validationErrors, validator.Validate()...)
	}
	if p.app.Validate != nil {
		validationErrors = appendNonNil(validationErrors, p.app.Validate(p.target.Addr().Interface())...)
	}
	if len(validationErrors) > 0 {
		return fmt.Errorf("argweave: configuration validation failed: %w", errors.Join(validationErrors...))
	}
	return nil
}

func appendNonNil(dst []error, src ...error) []error {
	for _, err := range src {
		if err != nil {
			dst = append(dst, err)
		}
	}
	return dst
}

// ConfigJSON returns the currently resolved configuration as indented
// JSON, keyed the same way as a ProviderFile config file (long flag name,
// or kebab-case field name for positional fields) so the output can be
// fed back in as a --config file. time.Duration and custom Value fields
// are rendered via their String() form instead of their raw internal
// representation. Callers typically use it after Parse returns
// COMMAND_PRINT_CONFIG, to write the output wherever they see fit; the
// library itself never writes to stdout/stderr or logs anything.
func (p *Parser) ConfigJSON() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m := make(map[string]interface{}, len(p.entries))
	for _, e := range p.entries {
		v := exportValue(e.value(p.target), e)
		if isEmptyExportValue(v) {
			continue
		}
		m[e.configKey()] = v
	}
	return json.MarshalIndent(m, "", "    ")
}

// isEmptyExportValue reports whether v is the zero value for its type
// (nil, "", false, 0, or an empty slice/map), so ConfigJSON can omit it.
func isEmptyExportValue(v interface{}) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String:
		return rv.Len() == 0
	case reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() == 0
	case reflect.Bool:
		return !rv.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() == 0
	case reflect.Ptr, reflect.Interface:
		return rv.IsNil()
	default:
		return false
	}
}

// DumpConfig returns the resolved configuration as indented JSON. It is a
// convenience wrapper around ConfigJSON that ignores serialization errors.
func (p *Parser) DumpConfig() string {
	bytes, _ := p.ConfigJSON()
	return string(bytes)
}

// Sources returns, for every field that was resolved on the last Parse
// call, which provider supplied its value (keyed like ConfigJSON/config
// files). Useful for debugging or for annotating --print-config output.
func (p *Parser) Sources() map[string]ProviderKind {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]ProviderKind, len(p.resolvedFrom))
	for e, kind := range p.resolvedFrom {
		out[e.configKey()] = kind
	}
	return out
}

// SourcesDetailed returns provider diagnostics for every field resolved by
// the last Parse call, including the source key, config-file path when
// applicable, and whether the value is marked secret.
func (p *Parser) SourcesDetailed() map[string]SourceInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]SourceInfo, len(p.resolvedInfo))
	for e, info := range p.resolvedInfo {
		out[e.configKey()] = info
	}
	return out
}

func exportValue(field reflect.Value, e *entry) interface{} {
	if e.Secret {
		return REDACTED_PLACEHOLDER
	}
	if e.isCustom {
		if field.Kind() == reflect.Slice {
			values := make([]interface{}, field.Len())
			for i := 0; i < field.Len(); i++ {
				values[i] = exportScalarValue(field.Index(i))
			}
			return values
		}
		return exportScalarValue(field)
	}
	return field.Interface()
}

func exportScalarValue(field reflect.Value) interface{} {
	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			return nil
		}
		if v, ok := field.Interface().(Value); ok {
			return v.String()
		}
		if tu, ok := field.Interface().(encoding.TextMarshaler); ok {
			if data, err := tu.MarshalText(); err == nil {
				return string(data)
			}
		}
	} else if field.CanAddr() {
		if v, ok := field.Addr().Interface().(Value); ok {
			return v.String()
		}
		if tu, ok := field.Addr().Interface().(encoding.TextMarshaler); ok {
			if data, err := tu.MarshalText(); err == nil {
				return string(data)
			}
		}
	}
	if field.Type() == durationType {
		return field.Interface().(time.Duration).String()
	}
	return fmt.Sprint(field.Interface())
}

// assignPositional distributes leftover positional arguments across the
// fields tagged `positional`, in struct declaration order. If the last
// positional field is a slice, it collects every remaining argument.
func (p *Parser) assignPositional(positional []string, values map[*entry]string, sliceAcc map[*entry][]string, setFromCLI map[*entry]bool) error {
	idx := 0
	for _, e := range p.posArgs {
		if e.isSlice {
			sliceAcc[e] = append(sliceAcc[e], positional[idx:]...)
			idx = len(positional)
			setFromCLI[e] = true
			break
		}
		if idx >= len(positional) {
			continue
		}
		values[e] = positional[idx]
		setFromCLI[e] = true
		idx++
	}
	if idx < len(positional) {
		return fmt.Errorf("argweave: unexpected extra positional argument(s): %s", strings.Join(positional[idx:], " "))
	}
	return nil
}

func (e *entry) flagLabel() string {
	switch {
	case e.Positional:
		return "<" + e.positionalName() + ">"
	case e.Long != "" && e.Short != "":
		return fmt.Sprintf("-%s/--%s", e.Short, e.Long)
	case e.Long != "":
		return "--" + e.Long
	default:
		return "-" + e.Short
	}
}

func (e *entry) positionalName() string {
	if e.ValueNameInHelpDescription != "" {
		return strings.ToUpper(e.ValueNameInHelpDescription)
	}
	return strings.ToUpper(e.fieldName)
}

// configKey is the key used to look up a field's value inside a decoded
// JSON/TOML config file: its long flag name, or a kebab-case version of
// the Go field name for positional-only fields.
func (e *entry) configKey() string {
	return e.configKeys()[0]
}

// configKeys returns every key that resolves this field from a config
// file, in priority order: the long flag name (or a kebab-case field name
// for positional-only fields) followed by any aliases.
func (e *entry) configKeys() []string {
	if e.Long == "" {
		return []string{camelToKebab(e.fieldName)}
	}
	keys := make([]string, 0, 1+len(e.Aliases))
	keys = append(keys, e.Long)
	keys = append(keys, e.Aliases...)
	return keys
}

func setValue(field reflect.Value, e *entry, val string) error {
	if e.FromFile {
		data, err := os.ReadFile(val)
		if err != nil {
			return fmt.Errorf("reading value file %q: %w", val, err)
		}
		val = strings.TrimSpace(string(data))
	}
	if e.isSlice {
		if val == "" {
			field.Set(reflect.MakeSlice(field.Type(), 0, 0))
			return nil
		}
		parts := strings.Split(val, ",")
		result := reflect.MakeSlice(field.Type(), 0, len(parts))
		for _, part := range parts {
			element := reflect.New(field.Type().Elem()).Elem()
			if err := setScalarValue(element, strings.TrimSpace(part)); err != nil {
				return err
			}
			result = reflect.Append(result, element)
		}
		field.Set(result)
		return nil
	}
	return setScalarValue(field, val)
}

func setScalarValue(field reflect.Value, val string) error {
	if isCustomType(field.Type()) {
		if field.Kind() == reflect.Ptr && field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		if field.Kind() == reflect.Ptr {
			if v, ok := field.Interface().(Value); ok {
				return v.Set(val)
			}
			if tu, ok := field.Interface().(encoding.TextUnmarshaler); ok {
				return tu.UnmarshalText([]byte(val))
			}
		} else if field.CanAddr() {
			if v, ok := field.Addr().Interface().(Value); ok {
				return v.Set(val)
			}
			if tu, ok := field.Addr().Interface().(encoding.TextUnmarshaler); ok {
				return tu.UnmarshalText([]byte(val))
			}
		}
		if field.Type() == durationType {
			d, err := time.ParseDuration(val)
			if err != nil {
				return err
			}
			field.SetInt(int64(d))
			return nil
		}
		return fmt.Errorf("unsupported custom type %s", field.Type())
	}

	if field.Kind() == reflect.Ptr {
		return fmt.Errorf("unsupported pointer type %s", field.Type())
	}
	switch field.Kind() {
	case reflect.String:
		field.SetString(val)
	case reflect.Bool:
		b, err := strconv.ParseBool(val)
		if err != nil {
			return err
		}
		field.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(val, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(val, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(val, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetFloat(f)
	default:
		return fmt.Errorf("unsupported field kind %s", field.Kind())
	}
	return nil
}

func isCustomType(t reflect.Type) bool {
	if t == durationType || t.Implements(valueType) || t.Implements(textUnmarshalerType) {
		return true
	}
	return t.Kind() != reflect.Ptr && (reflect.PointerTo(t).Implements(valueType) || reflect.PointerTo(t).Implements(textUnmarshalerType))
}

func isSupportedScalar(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// Version returns the "<name> <version>" line printed for --version/-V,
// for callers to write out wherever they see fit.
func (p *Parser) Version() string {
	name := p.app.Name
	if name == "" {
		name = "app"
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s", name, p.app.Version))
}

// GenerateHelp renders the --help page for the parser, delegating to
// AppConfig.HelpRenderer (DefaultHelpRenderer() unless overridden).
func (p *Parser) GenerateHelp() string {
	return p.app.HelpRenderer.RenderHelp(p)
}
