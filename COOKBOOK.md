# argweave Cookbook

Task-oriented recipes for things that come up once you're past the basic setup in [README.md](README.md). Each recipe is a question you might actually ask, followed by the short answer and a runnable example.

## Table of Contents

- [argweave Cookbook](#argweave-cookbook)
  - [Table of Contents](#table-of-contents)
  - [Documentation generation](#documentation-generation)
    - [How do I document several different structs in the same output file?](#how-do-i-document-several-different-structs-in-the-same-output-file)
    - [How do I document a struct whose sub-struct lives in another package?](#how-do-i-document-a-struct-whose-sub-struct-lives-in-another-package)
  - [Debugging and introspection](#debugging-and-introspection)
    - [How do I debug loaded values and understand which provider they come from?](#how-do-i-debug-loaded-values-and-understand-which-provider-they-come-from)
    - [How do I know how long parsing took?](#how-do-i-know-how-long-parsing-took)
  - [Validation](#validation)
    - [How can I validate my parameters?](#how-can-i-validate-my-parameters)
    - [How do I express "this flag needs that one" or "these two are exclusive"?](#how-do-i-express-this-flag-needs-that-one-or-these-two-are-exclusive)
  - [Flags and options](#flags-and-options)
    - [How do I turn off a boolean flag that defaults to `true`?](#how-do-i-turn-off-a-boolean-flag-that-defaults-to-true)
  - [Configuration sources](#configuration-sources)
    - [How do I change which source wins when a value is set in several places?](#how-do-i-change-which-source-wins-when-a-value-is-set-in-several-places)
    - [How do I point at a config file that isn't auto-detected?](#how-do-i-point-at-a-config-file-that-isnt-auto-detected)
  - [Secrets](#secrets)
    - [How do I keep a secret out of `--print-config` and logs?](#how-do-i-keep-a-secret-out-of---print-config-and-logs)
  - [Shell completion](#shell-completion)
    - [How do I ship shell completion to users?](#how-do-i-ship-shell-completion-to-users)


## Documentation generation

### How do I document several different structs in the same output file?

Use `-marker` to give each `weavedoc -edit` invocation its own pair of HTML markers, so multiple generated tables can coexist in the same Markdown file without overwriting each other.

Add one marker pair per struct to the target file:

```markdown
## Server options

<!-- server:start -->
<!-- server:end -->

## Database options

<!-- database:start -->
<!-- database:end -->
```

Then run `weavedoc` once per struct, pointing `-marker` at the matching label:

```bash
go run github.com/SR-G/argweave/cmd/weavedoc -type=ServerConfig -file=config.go -edit=CONFIG.md -marker=server
go run github.com/SR-G/argweave/cmd/weavedoc -type=DatabaseConfig -file=config.go -edit=CONFIG.md -marker=database
```

Each run only replaces the content between its own `<!-- <marker>:start -->` / `<!-- <marker>:end -->` pair, leaving the rest of the file untouched. Wire both invocations as separate `//go:generate` directives so `go generate ./...` regenerates both tables together.

### How do I document a struct whose sub-struct lives in another package?

`-file` accepts a comma-separated list of files and/or directories, which are all parsed together before the struct is flattened. Point it at the primary file plus wherever the nested struct is defined:

```bash
go run github.com/SR-G/argweave/cmd/weavedoc -type=Config -file=config.go,../shared/dbconfig -out=docs/CONFIG.md
```

This only resolves types that `weavedoc` can statically parse from source (plain Go files on disk); it cannot follow an import path into another module's compiled package. If the sub-struct is truly external (a third-party dependency, not source you can list a path to), it won't be flattened — the runtime parser flattens it fine via reflection, but `weavedoc` needs the source file to be reachable through `-file`.


## Debugging and introspection

### How do I debug loaded values and understand which provider they come from?

Call `Parser.Sources()` or `Parser.SourcesDetailed()` after `Parse` to see, per field, which provider actually supplied the value:

```go
if err := parser.Parse(os.Args[1:]); err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
}

fmt.Println(parser.Sources()["port"]) // e.g. "flags", "env", "file", or "default"

for key, info := range parser.SourcesDetailed() {
    fmt.Printf("%s: provider=%s key=%s detail=%q secret=%v\n",
        key, info.Provider, info.Key, info.Detail, info.Redacted)
}
```

`SourcesDetailed` is the richer of the two: alongside the `ProviderKind`, `SourceInfo` also reports the exact key that was looked up (e.g. the environment variable name, or the config file key), the config file path in `Detail` when the value came from a file, and whether the field is marked `secret`.

For a quick manual check without writing any code, the built-in `--print-config` flag dumps the fully resolved configuration as JSON (secrets redacted), which is the fastest way to confirm what a given combination of flags/env/file actually resolves to.

### How do I know how long parsing took?

`Parser.ParsingDuration()` returns the elapsed time of the most recent `Parse` call, including calls that returned help, version, or an error:

```go
_ = parser.Parse(os.Args[1:])
fmt.Println(parser.ParsingDuration())
```

It's zero until `Parse` has run at least once.


## Validation

### How can I validate my parameters?

argweave supports two kinds of custom, whole-configuration validators, both run after every provider has resolved its values and after type conversion:

1. **Implement `argweave.Validator` on the config struct itself**, when you own the type:

   ```go
   func (c *Config) Validate() []error {
       var errs []error
       if c.TLS && c.Insecure {
           errs = append(errs, errors.New("tls and insecure cannot both be enabled"))
       }
       return errs
   }
   ```

2. **Pass `AppConfig.Validate` as a callback**, when the struct can't (or shouldn't) implement the interface directly:

   ```go
   parser, err := argweave.New(&cfg, argweave.AppConfig{
       Validate: func(value interface{}) []error {
           config := value.(*Config)
           var errs []error
           if config.Port < 1024 {
               errs = append(errs, errors.New("port must be >= 1024"))
           }
           return errs
       },
   })
   ```

Both are equivalent in effect — use whichever fits how the config type is defined. If both are present, both run and their errors are joined together.

### How do I express "this flag needs that one" or "these two are exclusive"?

That's the lighter-weight, tag-level validation, for simple relationships between two fields rather than arbitrary cross-field logic:

```go
type Config struct {
    TLS         bool   `arg:"long=tls"`
    Certificate string `arg:"long=cert,requires=tls"`
    Insecure    bool   `arg:"long=insecure,conflicts=tls"`
}
```

`requires` and `conflicts` reference another field by Go field name, long flag, or alias, and are checked automatically after resolution — no `Validate` method needed for these simple cases.


## Flags and options

### How do I turn off a boolean flag that defaults to `true`?

Every boolean option automatically also accepts a `--no-<name>` variant (and `--no-<alias>` for each alias), with no extra tag needed:

```go
type Config struct {
    Verbose bool `arg:"long=verbose,default=true,help=Enable verbose logging"`
}
```

```bash
myapp --no-verbose
```

This is the only way to override a boolean that defaults to `true` from the command line, since passing `--verbose` again would be a no-op. The negated form is also listed automatically in `--help` and included by `weavedoc`/`GenerateCompletion` output.


## Configuration sources

### How do I change which source wins when a value is set in several places?

Set `AppConfig.Providers` to a reordered (or trimmed) slice. The first provider in the list that resolves a value for a field wins:

```go
// Flags win over environment variables, which win over the config file.
parser, err := argweave.New(&cfg, argweave.AppConfig{
    Providers: []argweave.ProviderKind{
        argweave.ProviderFlags, argweave.ProviderEnv, argweave.ProviderFile, argweave.ProviderDefault,
    },
})
```

Leaving `Providers` empty uses `argweave.DefaultProviders()` (env, then flags, then file, then default). Omit a `ProviderKind` from the slice to disable that source entirely.

### How do I point at a config file that isn't auto-detected?

Pass `-c` / `--config <path>` on the command line (only available when `ProviderFile` is enabled), or change where auto-detection looks via `AppConfig.ConfigSearchPaths`:

```go
parser, err := argweave.New(&cfg, argweave.AppConfig{
    ConfigSearchPaths: []string{"/etc/myapp", "."},
})
```

Without an explicit `-c`, argweave looks for `<binary-name>.json` then `<binary-name>.toml` in those directories.


## Secrets

### How do I keep a secret out of `--print-config` and logs?

Add `secret` to the field's tag:

```go
APIKey string `arg:"long=api-key,env=API_KEY,secret,help=API key for authentication"`
```

A secret field's value is replaced with a redacted placeholder in `Parser.ConfigJSON()` / `--print-config` output, and `SourceInfo.Redacted` is `true` for it in `SourcesDetailed()`, so debugging/introspection code can choose not to print the real value either.


## Shell completion

### How do I ship shell completion to users?

Two options, depending on whether completion should reflect flags known at runtime or be generated ahead of time:

- At runtime, call `Parser.GenerateCompletion(shell)` (`"bash"`, `"zsh"`, `"fish"`, or `"powershell"`) and wire it to a hidden flag or companion command.
- Ahead of time, have `weavedoc` emit a static completion script during `go generate`, so it can be installed without running your binary:

  ```bash
  go run github.com/SR-G/argweave/cmd/weavedoc -type=Config -file=config.go -completion=bash -completion-out=completions/myapp.bash -name=myapp
  ```
