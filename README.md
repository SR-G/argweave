# argweave

<p style="text-align: right">
  
— *“We weave our fate, one thread at a time.”* (a quote from the Norns, in Nordic Mythology)

— *“We weave our config, one arg at a time.”* (a quote from a random Go developer)
</p>


`argweave` is a small Go library for defining in a flexible way CLI flags, environment variable bindings, and file-based configuration, through struct tags. Hence avoiding to use a combination of multiple libraries doing a sub-set of these features, or a way heavier library.

- Define options once in a struct using tags.
- Map each option to a short flag (`-p`), a long flag (`--port`), one or more aliases, a positional argument, and/or an environment variable (`PORT`).
- Mark options as required, give them a default value, and add help text.
- Embed and flatten sub-structs to compose configs, and plug in custom types via a `flag.Value`-like interface, `encoding.TextUnmarshaler`, or `time.Duration`.
- Get fully formatted `--help` / `-h` output for free, plus `--print-config` to dump the resolved configuration as JSON.
- Load values from a JSON or TOML config file, auto-detected from the binary name or overridden with `-c` / `--config`.
- Generate Markdown documentation for your options with `go generate` (to automatically update a `README.md`, etc.).
- The default `Parse` method does not log or terminate the process; use `ParseAndHandleExitIfNeeded` as a wrapper, if you need to simplify the code in which you embed this library (see [examples/](examples/)).

See [COOKBOOK.md](COOKBOOK.md) for task-oriented recipes (debugging resolved values, validating parameters, documenting multiple structs, etc.).

Aside from TOML parsing, argweave is dependency-free.

## Table of Contents

- [argweave](#argweave)
  - [Table of Contents](#table-of-contents)
  - [Comparison with other libraries](#comparison-with-other-libraries)
    - [How to read the comparison](#how-to-read-the-comparison)
  - [Installation](#installation)
  - [Feature 1 — Defining options via struct tags](#feature-1--defining-options-via-struct-tags)
    - [Tag reference](#tag-reference)
    - [Supported field types](#supported-field-types)
    - [Nested sub-objects / flattening](#nested-sub-objects--flattening)
    - [Positional arguments and `--`](#positional-arguments-and---)
    - [Negatable booleans](#negatable-booleans)
    - [Flag name abbreviation](#flag-name-abbreviation)
    - [Common pitfalls and edge cases](#common-pitfalls-and-edge-cases)
    - [Value resolution order](#value-resolution-order)
    - [Cross-field validation](#cross-field-validation)
      - [Validation at tags level](#validation-at-tags-level)
      - [Global Custom Validation at GO struct level](#global-custom-validation-at-go-struct-level)
      - [Global Custom Validation through a callback](#global-custom-validation-through-a-callback)
  - [Feature 2 — Global configuration](#feature-2--global-configuration)
    - [Providers: choosing and ordering value sources](#providers-choosing-and-ordering-value-sources)
    - [Instanciation](#instanciation)
    - [Config files (`argweave.ProviderFile`)](#config-files-argweaveproviderfile)
    - [Inspecting resolved values](#inspecting-resolved-values)
  - [Feature 3 — `--help` / `-h`, `--version` / `-V`, and `--print-config`](#feature-3----help---h---version---v-and---print-config)
    - [Shell completion](#shell-completion)
  - [Feature 4 — Generating Markdown documentation with `go generate`](#feature-4--generating-markdown-documentation-with-go-generate)
    - [Full page mode (`-out`)](#full-page-mode--out)
    - [Edit mode (`-edit`)](#edit-mode--edit)
    - [JSON Schema mode (`-schema`)](#json-schema-mode--schema)
    - [Generated table example](#generated-table-example)
  - [Examples](#examples)
  - [Cookbook](COOKBOOK.md)
  - [Design notes / limitations](#design-notes--limitations)
  - [Dev Activities](#dev-activities)
    - [Regenerate the test MARKDOWN content](#regenerate-the-test-markdown-content)
  - [Releases](#releases)
  - [License](#license)


## Comparison with other libraries

Go has many excellent libraries in this space, but they solve slightly different problems. Some are configuration loaders, some are CLI parsers, and some are application frameworks that combine a parser with command dispatch. The table below compares their native capabilities; integrations and application code can extend several of the `No` cells.

<span style="color:green">✅</span> = supported natively &nbsp;&nbsp; <span style="color:red">❌</span> = not a native capability

| Library | CLI flags | ENV variables | File config | Struct config | Provider precedence | Subcommands | Shell completion | Documentation generation | Main focus |
|---------|-----------|----------------|-------------|----------------|---------------------|-------------|------------------|---------------------------|------------|
| **argweave** | <span style="color:green">✅</span> | <span style="color:green">✅</span> | <span style="color:green">✅</span> JSON/TOML | <span style="color:green">✅</span> struct tags | <span style="color:green">✅</span> ordered providers | <span style="color:red">❌</span> | <span style="color:green">✅</span> Bash/Zsh/Fish/PowerShell | <span style="color:green">✅</span> built-in `weavedoc` (Markdown + JSON Schema) | One tagged configuration model across CLI, ENV, files, and defaults |
| [Viper](https://github.com/spf13/viper) | <span style="color:green">✅</span> via pflag binding | <span style="color:green">✅</span> | <span style="color:green">✅</span> many formats | <span style="color:green">✅</span> via mapstructure | <span style="color:green">✅</span> documented precedence | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> | Application configuration hub |
| [Koanf](https://github.com/knadh/koanf) | <span style="color:green">✅</span> via providers | <span style="color:green">✅</span> | <span style="color:green">✅</span> provider-based | <span style="color:green">✅</span> unmarshalling | <span style="color:green">✅</span> load order | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> | Composable configuration loading |
| [Kong](https://github.com/alecthomas/kong) | <span style="color:green">✅</span> | <span style="color:green">✅</span> | <span style="color:red">❌</span> not a core file provider | <span style="color:green">✅</span> struct tags | <span style="color:green">✅</span> flag/env/default rules | <span style="color:green">✅</span> | <span style="color:green">✅</span> completion support | <span style="color:red">❌</span> | Struct-tag CLI parsing |
| [Cobra](https://github.com/spf13/cobra) | <span style="color:green">✅</span> via pflag | <span style="color:red">❌</span> manual binding/integration | <span style="color:red">❌</span> manual integration | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:green">✅</span> | <span style="color:green">✅</span> | <span style="color:green">✅</span> via companion `cobra/doc` package (Markdown/man/ReST) | CLI commands and application framework |
| [flaggy](https://github.com/integrii/flaggy) | <span style="color:green">✅</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:green">✅</span> | <span style="color:green">✅</span> | <span style="color:red">❌</span> | Lightweight CLI parsing |
| [go-flags](https://github.com/jessevdk/go-flags) | <span style="color:green">✅</span> | <span style="color:green">✅</span> via tags | <span style="color:red">❌</span> | <span style="color:green">✅</span> struct tags | <span style="color:red">❌</span> cross-provider ordering | <span style="color:green">✅</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> | Struct-based CLI parsing |
| [caarlos0/env](https://github.com/caarlos0/env) | <span style="color:red">❌</span> | <span style="color:green">✅</span> | <span style="color:red">❌</span> | <span style="color:green">✅</span> struct tags | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:green">✅</span> full compatibility via external tool (`envdoc`) | Environment-only configuration |
| [ilyakaznacheev/cleanenv](https://github.com/ilyakaznacheev/cleanenv) | <span style="color:red">❌</span> flag package integration is usage-text only | <span style="color:green">✅</span> | <span style="color:green">✅</span> YAML/JSON/TOML/EDN/ENV | <span style="color:green">✅</span> struct tags | <span style="color:green">✅</span> file, then ENV override, then default | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:green">✅</span> built-in `GetDescription`/`FUsage`, plus full compatibility via external `envdoc` | Environment + config-file reader |
| [envconfig](https://github.com/kelseyhightower/envconfig) | <span style="color:red">❌</span> | <span style="color:green">✅</span> | <span style="color:red">❌</span> | <span style="color:green">✅</span> struct tags | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> runtime `Usage()` only, no generated file | Environment-only configuration |
| [sethvargo/go-envconfig](https://github.com/sethvargo/go-envconfig) | <span style="color:red">❌</span> | <span style="color:green">✅</span> | <span style="color:red">❌</span> | <span style="color:green">✅</span> struct tags | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> partial compatibility via external tool (`envdoc`) | Environment-only configuration |
| [joeshaw/envdecode](https://github.com/joeshaw/envdecode) | <span style="color:red">❌</span> | <span style="color:green">✅</span> | <span style="color:red">❌</span> | <span style="color:green">✅</span> struct tags | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> partial compatibility via external tool (`envdoc`) | Environment-only configuration |
| [`flag`](https://pkg.go.dev/flag) | <span style="color:green">✅</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> | <span style="color:red">❌</span> | Go standard-library flags |

### How to read the comparison

- **Viper** and **Koanf** are the closest alternatives when the primary need is application configuration assembled from many sources. They generally use a map-based configuration model and a separate CLI parser or provider.
- **Kong**, **go-flags**, and **flaggy** are primarily CLI parsers. They are strong choices when command-line UX or subcommands matter more than a unified file/ENV provider pipeline.
- **Cobra** is an application and command framework rather than a complete configuration system. It commonly appears alongside Viper, pflag, or a custom configuration layer.
- **caarlos0/env**, **envconfig**, **sethvargo/go-envconfig**, and **joeshaw/envdecode** are deliberately narrower environment loaders. They are a good fit when configuration comes only from environment variables and a struct. **ilyakaznacheev/cleanenv** is similar but also reads configuration files (YAML/JSON/TOML/EDN/ENV).
- **argweave** focuses on one tagged struct being resolved through ordered ENV, flag, file, and default providers, while keeping output and process termination under the caller's control.
- Documentation generation is native (built into the library, like argweave's `weavedoc`, Cobra's `cobra/doc` package, or cleanenv's `GetDescription`/`FUsage`) in some cases, achievable only through a separate external tool (like [`envdoc`](https://github.com/g4s8/envdoc), which has full compatibility with caarlos0/env and cleanenv and partial compatibility with sethvargo/go-envconfig and joeshaw/envdecode) in others, and absent elsewhere.

No library is universally better. The important design choice is whether the application wants a CLI framework, a general configuration hub, an environment-only loader, or one configuration model shared by all supported input sources.

## Installation

```bash
go get github.com/SR-G/argweave
```

```go
import argweave "github.com/SR-G/argweave"
```

## Feature 1 — Defining options via struct tags

Annotate a regular Go struct with `arg` tags - a few examples :

```go
type Config struct {
    Port    int      `arg:"short=p,long=port,env=PORT,default=8080,help=Port number to listen on"`
    Host    string   `arg:"long=host,env=HOST,default=localhost,alias=hostname,help=Host address to bind to"`
    APIKey  string   `arg:"long=api-key,env=API_KEY,required,help=API key for authentication"`
    Verbose bool     `arg:"short,long,help=Enable verbose logging"`
    Tags    []string `arg:"long=tags,env=TAGS,help=Comma separated list of tags"`

    // Positional arguments: declared in order, no short/long flag.
    InputFile string   `arg:"positional,required,value_name=file,help=Input file to process"`
    ExtraArgs []string `arg:"positional,help=Additional passthrough arguments"`
}
```

### Tag reference

The `arg` tag is a comma-separated list of `key` or `key=value` tokens - full list is :

| Key                              | Description                                                                                                             |
| -------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `short`                          | Derive the short flag from the first letter of the field name (`-p`).                                                   |
| `short=x`                        | Use `x` as the short flag explicitly.                                                                                   |
| `long`                           | Derive a kebab-case long flag from the field name (`APIKey` → `--api-key`).                                             |
| `long=name`                      | Use `name` as the long flag explicitly.                                                                                 |
| `alias=name`                     | Register an additional long name for the flag; may repeat.                                                              |
| `env=NAME`                       | Fall back to environment variable `NAME` if the flag is not passed.                                                     |
| `default=value`                  | Value used when neither the flag nor the environment variable is set.                                                   |
| `required`                       | Parsing fails if no value can be resolved from CLI, env, or default.                                                    |
| `positional`                     | Bind the value from a positional argument instead of a flag (cannot be combined with `short` / `long` / `alias`).       |
| `hidden`                         | Omit the option from `--help` and from generated Markdown docs.                                                         |
| `value_name=NAME`                | Override the `<PLACEHOLDER>` shown in `--help` for this option.                                                         |
| `group=NAME`                     | Place the option under a named section in `--help`.                                                                     |
| `requires=FIELD`                 | Require another field to be active when this option is set; may repeat. Reference a Go field name, long flag, or alias. |
| `conflicts=FIELD`                | Reject this option when another field is active; may repeat. Reference a Go field name, long flag, or alias.            |
| `secret`                         | Redact the resolved value as `[REDACTED]` in `ConfigJSON()`.                                                            |
| `file` / `from_file`             | Treat the resolved value as a path and use the file contents as the option value.                                       |
| `help=text` / `description=text` | Description shown in `--help` and in the generated Markdown docs.                                                       |

A non-positional field must define at least a `short` or a `long` flag. A comma that must appear inside a value, such as inside `help=...`, can be escaped as `\,`.


### Supported field types

- `string`, `bool`, all signed and unsigned integer types, and `float32` / `float64`.
- Slices of supported scalar or custom types: accept comma-separated values (`--tags a,b,c`) and/or repeated flags (`--tags a --tags b`); both forms can be mixed. This includes `[]string`, `[]time.Duration`, and slices of types implementing `argweave.Value` or `encoding.TextUnmarshaler`.
- `time.Duration`, parsed with `time.ParseDuration` (for example, `--timeout 30s`).
- Any type, including pointer fields, whose value or pointer implements `argweave.Value` (mirroring the standard library's `flag.Value`: `String() string` and `Set(string) error`) or `encoding.TextUnmarshaler`; use this to plug in your own types such as URLs, enums, and IP addresses.

Numeric values are range-checked against the declared Go type, so values that do not fit in `uint8`, `int16`, `float32`, and similar types are rejected instead of being truncated.


### Nested sub-objects / flattening

Any struct-typed field with no `arg` tag of its own has its fields flattened into the parent automatically, letting you compose a config from reusable, independently testable pieces, whether embedded (anonymous) or referenced as a plain named field:

```go
type DatabaseConfig struct {
    Host string `arg:"long=db-host,env=DB_HOST,default=localhost,help=Database host"`
    Port int    `arg:"long=db-port,env=DB_PORT,default=5432,help=Database port"`
}

type Config struct {
    DB   DatabaseConfig // named field: contributes --db-host / --db-port
    Port int            `arg:"long=port,default=8080,help=Port"`
}
```

This works identically through every provider (CLI flags, environment variables, config files, defaults) and at any nesting depth, including a pointer to a struct (`*DatabaseConfig`, allocated automatically once a value is resolved for one of its fields). `weavedoc` (Feature 4) flattens nested sub-objects in the same way when generating documentation.

Recursive untagged struct graphs are rejected when the parser is created, because they cannot be flattened into a finite option set.


### Positional arguments and `--`

Fields tagged `positional` are filled, in struct declaration order, from arguments that are not flags. If the last positional field is a slice, it collects every remaining argument like a trailing variadic parameter. A literal `--` stops flag parsing entirely; everything after it is treated as positional, even if it looks like a flag.


### Negatable booleans

Every boolean flag automatically gets a `--no-<name>` form (and one per alias) to force it back to `false`, for example `--verbose --no-verbose`.


### Flag name abbreviation

Long flags may be abbreviated to any unambiguous prefix. For example, `--por 9090` resolves to `--port 9090` as long as no other flag or alias shares that prefix; an ambiguous prefix is a parse error listing the candidates.


### Common pitfalls and edge cases

A few behaviors are intentionally strict, and they are worth knowing up front because they usually surface as parse-time errors rather than silent misconfiguration:

- Ambiguous long flags: abbreviations like `--por` are allowed only when they match a single candidate. If more than one flag or alias shares the same prefix, parsing fails and the candidates are listed in the error.
- Alias and flag collisions: each long flag name must be unique across the whole flattened config tree. Reusing `--port` or an alias such as `--db-host` in two different fields is rejected when the parser builds its lookup tables.
- Flattening only happens for untagged struct fields: a named or embedded struct field is flattened automatically only when it has no `arg` tag of its own. If a sub-struct is tagged with `arg`, it is treated as one option instead of being flattened.
- Pointer-to-struct flattening is supported, but only when a value is actually needed: a nil pointer is allocated automatically the first time a nested field is resolved.
- Positional arguments cannot be mixed with flag names: a field tagged `positional` may not also define `short`, `long`, or `alias` values. Likewise, only the last positional field may be a slice, so it can absorb trailing arguments.
- Negated booleans are generated automatically: every boolean option also accepts `--no-<name>` (and `--no-<alias>`), which is useful for toggling a default `true` value back off.


### Value resolution order

For each option, the configured providers are tried in order and the first one to yield a value wins. See "Providers" under Feature 2 for the default order and how to change it. If none resolve a value, the field keeps its Go zero value unless `required` is set, in which case `Parse` returns an error.


### Cross-field validation

#### Validation at tags level

Fields can declare relationships directly in tags. For example, `requires=tls` makes a certificate invalid unless the `tls` option is active, while `conflicts=insecure` rejects incompatible options. References may use a Go field name, long flag, or alias.


#### Global Custom Validation at GO struct level

For rules that depend on the complete configuration and for spec, implement `argweave.Validator` on the configuration pointer:

```go
func (c *Config) Validate() []error {
    var validationErrors []error
    if c.TLS && c.Insecure {
        validationErrors = append(validationErrors, errors.New("tls and insecure cannot both be enabled"))
    }
    return validationErrors
}
```

Relationship checks and `Validate()` execution are done after provider resolution and value conversion. All non-nil errors returned by `Validate()` are joined and reported together.


#### Global Custom Validation through a callback

If the configuration type cannot be modified (in order to implement the `Validator` interface), one can provide the same rule through `AppConfig.Validate` :

```go
parser, err := argweave.New(&cfg, argweave.AppConfig{
    Validate: func(value interface{}) []error {
        config := value.(*Config)
        var validationErrors []error
        if config.Port < 1024 {
            validationErrors = append(validationErrors, errors.New("port must be >= 1024"))
        }
        if config.TLS && config.Insecure {
            validationErrors = append(validationErrors, errors.New("tls and insecure cannot both be enabled"))
        }
        return validationErrors
    },
})
```

All non-nil errors returned by the callback are joined and reported together.

Conversion errors identify the option and provider, for example `invalid value for --port from file: expected integer`.


## Feature 2 — Global configuration

`argweave.AppConfig` configures the program-wide metadata shown in `--help`:

```go
parser, err := argweave.New(&cfg, argweave.AppConfig{
    Name:        "myapp",
    Version:     "1.0.0",
    Description: "My application description.",
    EnvPrefix:   "MYAPP_", // optional: prefixes every field's env=NAME lookup
})
```

`DisableHelp` and `DisableVersion` (both `bool`) turn off the built-in `-h` / `--help` and `-V` / `--version` flags entirely if you want to define your own.

`EnvPrefix` is blank by default (no behavior change). Once set, it is prepended to every field's `env=NAME` lookup, including fields flattened from nested sub-objects. For example, with `EnvPrefix: "MYAPP_"`, a `DatabaseConfig.Port` field tagged `env=DB_PORT` is read from `MYAPP_DB_PORT`.

An environment variable is considered provided only when it exists and is non-empty. An explicitly empty value therefore falls through to the next configured provider, such as a config file or default value.

### Providers: choosing and ordering value sources

Each field's value is resolved by trying a list of providers in order until one produces a value. There are four built-in providers:

| Provider                   | Resolves from                                |
| -------------------------- | -------------------------------------------- |
| `argweave.ProviderEnv`     | The field's `env=NAME` environment variable. |
| `argweave.ProviderFlags`   | Command-line flags and positional arguments. |
| `argweave.ProviderFile`    | A JSON or TOML config file.                  |
| `argweave.ProviderDefault` | The field's `default=value` tag.             |

The default order, used whenever `AppConfig.Providers` is left empty, is `argweave.DefaultProviders()` = Env, then Flags, then config File, then Default. So an environment variable overrides a command-line flag, which in turn overrides the config file, if more than one is set for the same field. Set `AppConfig.Providers` to reorder or drop a source entirely:

```go
// Command-line flags take precedence over environment variables.
parser, err := argweave.New(&cfg, argweave.AppConfig{
  Providers: []argweave.ProviderKind{argweave.ProviderFlags, argweave.ProviderEnv, argweave.ProviderFile, argweave.ProviderDefault},
})

// Ignore environment variables and config files entirely.
parser, err := argweave.New(&cfg, argweave.AppConfig{
  Providers: []argweave.ProviderKind{argweave.ProviderFlags, argweave.ProviderDefault},
})
```

### Instanciation 

`New` returns an error if `Providers` contains an unknown or duplicate entry.

Set `AppConfig.HelpRenderer` to replace the built-in help layout. A renderer receives the parser and can inspect its public rendering methods or maintain an application-specific format:

```go
type compactHelp struct{}

func (compactHelp) RenderHelp(parser *argweave.Parser) string {
    return parser.Version()
}

parser, err := argweave.New(&cfg, argweave.AppConfig{
    HelpRenderer: compactHelp{},
})
```

The process-level wrapper uses `AppConfig.ExitCodes`. Customize the standard mapping when an application follows a different CLI exit-code convention:

```go
codes := argweave.DefaultExitCodes()
codes.Register(64, argweave.EXIT_CODE_PURPOSE_OPTIONS_IN_ERROR)
parser, err := argweave.New(&cfg, argweave.AppConfig{ExitCodes: codes})
```

Set `AppConfig.StrictConfig` to reject unknown keys in JSON or TOML config files instead of silently ignoring them. Known long flag names, aliases, and positional field names are accepted:

```go
parser, err := argweave.New(&cfg, argweave.AppConfig{
    StrictConfig: true,
})
```

With strict mode enabled, a config containing a typo such as `porrt` fails with an error listing the unknown key. The default is `false` for backward compatibility.


### Config files (`argweave.ProviderFile`)

When `ProviderFile` is enabled, which it is by default, values can also come from a JSON or TOML file:

- Auto-detected filename: `<binary-name>.json`, then `<binary-name>.toml`, looked up in the current working directory. For example, a program built as `myapp` looks for `myapp.json` / `myapp.toml`.
- Explicit override: pass `-c` / `--config <path>` on the command line. This flag only exists when `ProviderFile` is enabled, and the file format is selected from its extension (`.json` or `.toml`).

The file is decoded into a plain key/value map. Each field is looked up by its long flag name (or a kebab-case version of the Go field name for positional-only fields), and its `alias=` names are also tried, so a config file mirrors the flag names shown in `--help`:

```json
{
  "port": 9090,
  "api-key": "secret",
  "tags": ["a", "b"]
}
```

```toml
port = 9090
api-key = "secret"
tags = ["a", "b"]
```

JSON numbers are decoded with `json.Number` (not `float64`), so large integers round-trip exactly instead of losing precision.

By default, the auto-detected filename is looked up in the current working directory and then in the directory containing the running binary (`providers.DefaultSearchDirs()`). Override this with `AppConfig.ConfigSearchPaths`:

```go
parser, err := argweave.New(&cfg, argweave.AppConfig{
    ConfigSearchPaths: []string{"/etc/myapp", "."},
})
```

Parsing TOML uses the [`github.com/BurntSushi/toml`](https://github.com/BurntSushi/toml) package, argweave's only third-party dependency, needed because the Go standard library has no TOML support. JSON decoding uses the standard library's `encoding/json`. The resolution logic for every provider (env, flags, file, default) lives in the internal `providers/` package, one file per provider (`providers/env.go`, `providers/flags.go`, `providers/file.go`, and `providers/default.go`).


### Inspecting resolved values

`Parser.ConfigJSON()` renders the fully resolved configuration using the same keys as a config file (long flag name or kebab-case field name), so its output can be fed back in as a `--config` file. `time.Duration` and custom `Value` fields are rendered via their `String()` form rather than their raw internal representation. `Parser.Sources()` reports, per key, which `ProviderKind` actually supplied the value, which is handy for debugging:

```go
_ = parser.Parse(os.Args[1:])
data, _ := parser.ConfigJSON()
fmt.Println(string(data))
fmt.Println(parser.Sources()["port"]) // e.g. "flags"
```

`Parser.ParsingDuration()` returns the elapsed time recorded by the most recent `Parse` call, including parses that return help, version, or an error. It returns zero before the parser has been used.

`Parser.SourcesDetailed()` provides richer diagnostics than `Sources()`: for each resolved field it reports the provider, source key, config-file path when applicable, and whether the field is marked secret.

For applications that want the library to render built-in command output without terminating the process, `Parser.Handle(args, stdout, stderr)` writes to the supplied writers and returns the configured exit code. For a conventional CLI, `Parser.ParseAndHandleExitIfNeeded(args)` is the convenience wrapper that renders the output and calls `os.Exit` for built-in commands or parse errors; this is the only library method that terminates the process.


## Feature 3 — `--help` / `-h`, `--version` / `-V`, and `--print-config`

`Parser.Parse` automatically recognizes `-h` / `--help`, `-V` / `--version`, and `--print-config` unless your struct explicitly claims one of those flags itself, in which case your definition wins and the built-in one is skipped for that token.

> **Parsing never writes to stdout or stderr or logs anything itself.** `Parse` returns a `Command` enum (`COMMAND_HELP`, `COMMAND_VERSION`, `COMMAND_PRINT_CONFIG`) plus a plain error. Use `Parser.Handle(args, stdout, stderr)` when the caller must retain process control, or use `Parser.ParseAndHandleExitIfNeeded(args)` as the convenience wrapper for a conventional CLI.

```go
func main() {
    var cfg Config

    parser, err := argweave.New(&cfg, argweave.AppConfig{
        Name:        "myapp",
        Version:     "1.0.0",
        Description: "My application description.",
    })
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        return
    }

    parser.ParseAndHandleExitIfNeeded(os.Args[1:])

    fmt.Printf("%+v\n", cfg)
}
```

Running `myapp --help` prints something like:

```text
basic-example 1.0.0
Example program demonstrating argweave.

USAGE:
    basic-example [OPTIONS] <FILE> [EXTRAARGS]...

ARGS:
    <FILE>          Input file to process [required]
    [EXTRAARGS]...  Additional passthrough arguments

OPTIONS:
        --api-key <API_KEY>  API key for authentication [env: API_KEY] [required]
        --host <HOST>        Host address to bind to [aliases: hostname] [env: HOST] [default: localhost]
        --tags <TAGS>        Comma separated list of tags [env: TAGS]
        --timeout <TIMEOUT>  Request timeout [default: 30s]
    -p, --port <PORT>        Port number to listen on [env: PORT] [default: 8080]
    -v, --verbose            Enable verbose logging
    -h, --help               Print help information
    -V, --version            Print version information
        --print-config       Print the resolved configuration as JSON
```

### Shell completion

`Parser.GenerateCompletion(shell)` renders a completion script for `"bash"`, `"zsh"`, `"fish"`, or `"powershell"`, listing every non-hidden flag and its aliases. As with everything else, it only returns the script text; writing it to a file or stdout is up to the caller:

```go
script, err := parser.GenerateCompletion("bash")
if err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
}
fmt.Println(script)
```

A typical program wires this to a hidden flag or a small companion command, such as `myapp --generate-completion=bash > /etc/bash_completion.d/myapp`.

The `weavedoc` generator can also create static completion files during `go generate`:

```bash
go run ./cmd/weavedoc -type=Config -file=config.go -completion=powershell -completion-out=docs/myapp.ps1 -name=myapp
```



## Feature 4 — Generating Markdown documentation with `go generate`

The `cmd/weavedoc` tool statically parses your Go source via `go/ast`, without reflecting or compiling your package, to extract the `arg` tags of a struct and render a Markdown table with each option's group, Go type, flags, aliases, positional status, environment variable, required/default metadata, relationships, behavior markers, and description. Fields marked `hidden` are omitted, and anonymous (embedded) struct fields are flattened just like at runtime. `-file` may also point to a directory, in which case every `*.go` file in it except `_test.go` is parsed, which is useful when the embedded sub-struct lives in a sibling file of the same package.

The generated table also includes the Go type, help group, `requires` and `conflicts` relationships, and behavior markers for `secret` and file-backed values. Markdown cell content is escaped so descriptions containing pipes or line breaks remain valid tables. Recursive flattened structs and duplicate type declarations are rejected during generation.

Use `-fields` to generate documentation with a selected, comma-separated list of Markdown columns. Supported names are `group`, `type`, `long`, `short`, `aliases`, `positional`, `env`, `required`, `default`, `requires`, `conflicts`, `behavior`, and `description`. The requested column order is preserved, duplicate names are ignored, and unknown names return an error. An empty value displays all columns. This option affects Markdown output; JSON Schema always contains its complete metadata:

```bash
go run github.com/SR-G/argweave/cmd/weavedoc -type=Config -file=config.go -fields=long,short,required,description -out=docs/CONFIG.md
go run github.com/SR-G/argweave/cmd/weavedoc -type=Config -file=config.go -schema=docs/config.schema.json
```

Add a `//go:generate` directive next to your struct:

```go
//go:generate go run github.com/SR-G/argweave/cmd/weavedoc -type=Config -file=config.go -out=docs/CONFIG.md
type Config struct {
    Port int `arg:"short=p,long=port,env=PORT,default=8080,help=Port number to listen on"`
    // ...
}
```

Then run:

```bash
go generate ./...
```

### Full page mode (`-out`)

Writes a complete, standalone Markdown page with an `# <title>` header to the given path.

```bash
go run github.com/SR-G/argweave/cmd/weavedoc -type=Config -file=config.go -out=docs/CONFIG.md -title="Configuration Reference"
```

### Edit mode (`-edit`)

Injects the generated table into an existing Markdown file, such as your `README.md`, between two HTML marker comments that you add once:

```markdown
## Configuration

<!-- weavedoc:start -->
<!-- weavedoc:end -->
```

```bash
go run github.com/SR-G/argweave/cmd/weavedoc -type=Config -file=config.go -edit=README.md
```

Everything between the two markers is replaced on every run, so the markers themselves must stay untouched.

### JSON Schema mode (`-schema`)

Writes a JSON Schema document for editor and external validation tooling. Built-in Go scalar types map to JSON Schema types, slices become arrays, required fields are listed in `required`, and argweave-specific details are retained as `x-argweave-*` extensions:

```bash
go run github.com/SR-G/argweave/cmd/weavedoc -type=Config -file=config.go -schema=docs/config.schema.json
```

Use `-strict-schema` to emit `additionalProperties: false`. Fields marked both `required` and `default` are not emitted as JSON Schema required properties because the runtime default satisfies them.


### Generated table example

| Group    | Type          | Flag          | Short | Aliases      | Positional | Env       | Required | Default     | Requires | Conflicts | Behavior | Description                      |
| -------- | ------------- | ------------- | ----- | ------------ | ---------- | --------- | -------- | ----------- | -------- | --------- | -------- | -------------------------------- |
| Server   | int           | `--port`      | `-p`  | -            | No         | `PORT`    | No       | `8080`      | -        | -         | -        | Port number to listen on         |
| Server   | string        | `--host`      | -     | `--hostname` | No         | `HOST`    | No       | `localhost` | -        | -         | -        | Host address to bind to          |
| Security | string        | `--api-key`   | -     | -            | No         | `API_KEY` | Yes      | -           | -        | -         | secret   | API key for authentication       |
| Server   | bool          | `--verbose`   | `-v`  | -            | No         | -         | No       | -           | -        | -         | -        | Enable verbose logging           |
| Runtime  | []string      | `--tags`      | -     | -            | No         | `TAGS`    | No       | -           | -        | -         | -        | Comma separated list of tags     |
| Runtime  | time.Duration | `--timeout`   | -     | -            | No         | -         | No       | `30s`       | -        | -         | -        | Request timeout                  |
| -        | string        | `<FILE>`      | -     | -            | Yes        | -         | Yes      | -           | -        | -         | -        | Input file to process            |
| -        | []string      | `<EXTRAARGS>` | -     | -            | Yes        | -         | No       | -           | -        | -         | -        | Additional passthrough arguments |

If a field has no `help=` value, `weavedoc` falls back to the field's Go doc comment (the comment directly above the field).


## Examples

See [examples/basic/main.go](examples/basic/main.go) for a runnable example, including its own `go:generate` directive and generated [examples/basic/CONFIG.md](examples/basic/CONFIG.md).

For a configuration that exercises the complete feature set, see [examples/full/main.go](examples/full/main.go) and its generated [examples/full/CONFIG.md](examples/full/CONFIG.md). It demonstrates nested configuration flattening, provider precedence, grouped help, custom values and slices, cross-field validation, secrets, file-backed values, positional arguments, and explicit exit management around `Parse()`:

```bash
go run ./examples/full --help
FULLAPP_API_KEY=development-key go run ./examples/full --insecure --mode batch input.txt
FULLAPP_API_KEY=development-key go run ./examples/full --tls --certificate server.crt input.txt
FULLAPP_API_KEY=development-key go run ./examples/full --print-config input.txt
```

```bash
go run ./examples/basic --help
go run ./examples/basic --port 9090 --verbose --api-key secret input.txt
PORT=9090 API_KEY=secret go run ./examples/basic input.txt
go run ./examples/basic --api-key secret input.txt --print-config
```


## Design notes / limitations

- **The parser does not print or log anything itself.** `Parse` returns a command value such as `COMMAND_HELP`, `COMMAND_VERSION`, or `COMMAND_PRINT_CONFIG` alongside any parse error. `ParseAndHandleExitIfNeeded` is the convenience wrapper that renders built-in output and exits. The standalone `cmd/weavedoc` tool and the example under `examples/` are separate CLI programs and own their process behavior.
- No subcommands: this library focuses purely on a single flat (or flattened) set of flags, positional arguments, and environment variables, matching the scope of a configuration library.
- `weavedoc` only looks at the AST of the given file or files, so the target struct (and any embedded sub-structs) must be defined, not just referenced, in `-file` or in the directory it points at.
- Runtime flags, environment handling, help rendering, and config parsing use only standard library packages (`reflect`, `os`, `strconv`, `strings`, `time`, `encoding`, and `encoding/json`); `go/ast`, `go/parser`, and `go/token` are used by `weavedoc`. `github.com/BurntSushi/toml` is the sole third-party dependency, used only for TOML config files.
- Somehow inspired by the [Rust clap crate](https://docs.rs/clap).


## Dev Activities

### Regenerate the test MARKDOWN content

In order to refresh the content in `examples/basic/CONFIG.md`, use : 

```shell
go generate ./...
```


## Releases

Releases are created automatically by [.github/workflows/release.yml](.github/workflows/release.yml) when a semantic-version tag is pushed to GitHub. The workflow checks formatting, generated documentation, tests, and `go vet` before creating a GitHub Release with automatically generated release notes.

Before the first GitHub release, update the module path in `go.mod` and the imports/examples to the final GitHub repository path. Go consumers use that module path when running `go get`.

From a clean, tested `master` branch:

```bash
go test ./...
go vet ./...
go generate ./...
git diff --exit-code

TAG="0.1.2"
git checkout master
git pull --ff-only origin master
git tag -a v${TAG} -m "Release v${TAG}$"
git push origin v${TAG}
```

The tag push starts the release workflow. After the verification job succeeds, GitHub creates the release for `v0.1.0` and fills its notes from merged pull requests and commit history. The workflow accepts tags matching `vMAJOR.MINOR.PATCH`, such as `v1.0.0` or `v1.2.3`.



## License

argweave is licensed under the [Apache License 2.0](LICENSE).
