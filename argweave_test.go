package argweave_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SR-G/argweave"
)

// testConfig exercises every feature the library supports: short/long
// flags, environment variable fallback, required fields, default values
// and slice values.
type testConfig struct {
	Port    int      `arg:"short=p,long=port,env=TESTAPP_PORT,default=8080,help=Port to listen on"`
	Host    string   `arg:"long=host,default=localhost,help=Host to bind"`
	APIKey  string   `arg:"long=api-key,env=TESTAPP_API_KEY,required,help=API key for auth"`
	Verbose bool     `arg:"short,long,help=Enable verbose logging"`
	Tags    []string `arg:"long=tags,help=Tags"`
}

func TestNumericWidthsAndCustomCollections(t *testing.T) {
	var cfg widthConfig
	parser, err := argweave.New(&cfg, argweave.AppConfig{Providers: []argweave.ProviderKind{argweave.ProviderFlags}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := parser.Parse([]string{"--small", "255", "--signed", "127", "--ratio", "1.5", "--token", "one", "--tokens", "two", "--tokens", "three", "--pointer-tokens", "four,five", "--durations", "1s,2s"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Small != 255 || cfg.Signed != 127 || cfg.Ratio != 1.5 {
		t.Fatalf("unexpected numeric values: %+v", cfg)
	}
	if cfg.Token == nil || cfg.Token.String() != "parsed:one" {
		t.Fatalf("unexpected pointer custom value: %+v", cfg.Token)
	}
	if len(cfg.Tokens) != 2 || cfg.Tokens[0].String() != "parsed:two" || cfg.Tokens[1].String() != "parsed:three" {
		t.Fatalf("unexpected custom slice: %+v", cfg.Tokens)
	}
	if len(cfg.PointerTokens) != 2 || cfg.PointerTokens[0].String() != "parsed:four" || cfg.PointerTokens[1].String() != "parsed:five" {
		t.Fatalf("unexpected pointer custom slice: %+v", cfg.PointerTokens)
	}
	if len(cfg.Durations) != 2 || cfg.Durations[0] != time.Second || cfg.Durations[1] != 2*time.Second {
		t.Fatalf("unexpected duration slice: %+v", cfg.Durations)
	}

	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "uint8 overflow", args: []string{"--small", "256"}},
		{name: "int8 overflow", args: []string{"--signed", "128"}},
		{name: "float32 overflow", args: []string{"--ratio", "3.5e38"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var invalid widthConfig
			parser, err := argweave.New(&invalid, argweave.AppConfig{Providers: []argweave.ProviderKind{argweave.ProviderFlags}})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if _, err := parser.Parse(test.args); err == nil {
				t.Fatalf("Parse(%v) unexpectedly succeeded", test.args)
			}
		})
	}
}

func TestRecursiveStructFlatteningIsRejected(t *testing.T) {
	var cfg recursiveConfig
	if _, err := argweave.New(&cfg, argweave.AppConfig{}); err == nil {
		t.Fatal("expected recursive struct flattening to be rejected")
	}
}

func TestHandleWritesWithoutExiting(t *testing.T) {
	var cfg testConfig
	parser, err := argweave.New(&cfg, argweave.AppConfig{Name: "testapp"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if code := parser.Handle([]string{"--help"}, &stdout, &stderr); code != argweave.OS_EXIT_OK {
		t.Fatalf("Handle help returned %d", code)
	}
	if !strings.Contains(stdout.String(), "--help") || stderr.Len() != 0 {
		t.Fatalf("unexpected help output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestEmptyEnvironmentFallsThrough(t *testing.T) {
	t.Setenv("ARGWEAVE_EMPTY", "")
	var cfg struct {
		Value string `arg:"long=value,env=ARGWEAVE_EMPTY,default=fallback"`
	}
	parser, err := argweave.New(&cfg, argweave.AppConfig{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := parser.Parse(nil); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Value != "fallback" {
		t.Fatalf("expected empty environment value to fall through, got %q", cfg.Value)
	}
}

func TestStrictConfigRejectsUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"name":"valid","unknown-z":1,"unknown-a":2}`), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	var strict fileConfig
	parser, err := argweave.New(&strict, argweave.AppConfig{StrictConfig: true})
	if err != nil {
		t.Fatalf("New strict: %v", err)
	}
	if _, err := parser.Parse([]string{"--config", path}); err == nil || !strings.Contains(err.Error(), "unknown-a, unknown-z") {
		t.Fatalf("expected sorted unknown-key error, got %v", err)
	}

	var permissive fileConfig
	parser, err = argweave.New(&permissive, argweave.AppConfig{})
	if err != nil {
		t.Fatalf("New permissive: %v", err)
	}
	if _, err := parser.Parse([]string{"--config", path}); err != nil {
		t.Fatalf("permissive config unexpectedly failed: %v", err)
	}
	if permissive.Name != "valid" {
		t.Fatalf("expected known config key to resolve, got %q", permissive.Name)
	}
}

func TestStrictConfigAcceptsAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"n":"alias-value"}`), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	var cfg fileConfig2
	parser, err := argweave.New(&cfg, argweave.AppConfig{StrictConfig: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := parser.Parse([]string{"--config", path}); err != nil {
		t.Fatalf("strict alias config failed: %v", err)
	}
	if cfg.Name != "alias-value" {
		t.Fatalf("expected alias to resolve, got %q", cfg.Name)
	}
}

// networkConfig is embedded (anonymous) below to exercise struct flattening.
type networkConfig struct {
	Bind string `arg:"long=bind,default=0.0.0.0,help=Bind address"`
}

// flattenConfig has an embedded, untagged struct field that should be
// flattened into the parent's flag set, plus positional arguments and a
// custom Value type.
type flattenConfig struct {
	networkConfig
	Verbose bool        `arg:"short,long,alias=noisy,help=Verbose logging"`
	Token   customValue `arg:"long=token,help=Custom-parsed token"`
	File    string      `arg:"positional,required,help=Input file"`
	Rest    []string    `arg:"positional,help=Remaining passthrough args"`
}

// customValue demonstrates the Value extension point for custom types.
type customValue struct{ raw string }

func (c *customValue) String() string     { return c.raw }
func (c *customValue) Set(s string) error { c.raw = "parsed:" + s; return nil }

// TestParserEndToEnd is the single high-level test for this library: it
// walks through flag parsing, environment variable fallback, default
// values, required-field validation, --help output, embedded-struct
// flattening, positional arguments, aliases, negatable booleans, custom
// types and --print-config, mirroring how a real consumer would use it.
func TestParserEndToEnd(t *testing.T) {
	app := argweave.AppConfig{Name: "testapp", Version: "0.0.1", Description: "test program"}

	// 1. CLI flags are resolved, including short flags and slices.
	var cfg testConfig
	p, err := argweave.New(&cfg, app)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := p.Parse([]string{"--port", "9090", "--api-key", "secret", "-v", "--tags", "a,b"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Port != 9090 || cfg.APIKey != "secret" || !cfg.Verbose || cfg.Host != "localhost" {
		t.Fatalf("unexpected cfg after CLI parse: %+v", cfg)
	}
	if len(cfg.Tags) != 2 || cfg.Tags[0] != "a" || cfg.Tags[1] != "b" {
		t.Fatalf("unexpected tags: %+v", cfg.Tags)
	}

	// 2. Environment variables are used as a fallback when a flag is
	// not set on the command line, and defaults apply otherwise.
	os.Setenv("TESTAPP_PORT", "7000")
	os.Setenv("TESTAPP_API_KEY", "env-secret")
	t.Cleanup(func() {
		os.Unsetenv("TESTAPP_PORT")
		os.Unsetenv("TESTAPP_API_KEY")
	})

	var cfg2 testConfig
	p2, err := argweave.New(&cfg2, app)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := p2.Parse(nil); err != nil {
		t.Fatalf("Parse with env fallback: %v", err)
	}
	if cfg2.Port != 7000 || cfg2.APIKey != "env-secret" || cfg2.Host != "localhost" {
		t.Fatalf("unexpected cfg after env fallback: %+v", cfg2)
	}

	// 3. A missing required field with no CLI value, no env value and
	// no default produces an error.
	os.Unsetenv("TESTAPP_API_KEY")
	var cfg3 testConfig
	p3, err := argweave.New(&cfg3, app)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := p3.Parse(nil); err == nil {
		t.Fatal("expected error for missing required --api-key, got nil")
	}

	// 4. --help is recognized, returns COMMAND_HELP and renders every flag.
	os.Setenv("TESTAPP_API_KEY", "env-secret")
	var cfg4 testConfig
	p4, err := argweave.New(&cfg4, app)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if command, err := p4.Parse([]string{"--help"}); command != argweave.COMMAND_HELP || err != nil {
		t.Fatalf("expected CommandHelp, got command=%v err=%v", command, err)
	}
	help := p4.GenerateHelp()
	for _, want := range []string{"testapp", "--port", "--api-key", "--verbose", "--tags", "--help", "--version"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help output missing %q:\n%s", want, help)
		}
	}

	// 5. Embedded struct flattening, aliases, negatable booleans,
	// positional arguments (including a trailing slice) and a custom
	// Value type all work together.
	var fcfg flattenConfig
	fp, err := argweave.New(&fcfg, app)
	if err != nil {
		t.Fatalf("New (flatten): %v", err)
	}
	if _, err := fp.Parse([]string{"--bind", "127.0.0.1", "--noisy", "--token", "abc", "input.txt", "extra1", "extra2"}); err != nil {
		t.Fatalf("Parse (flatten): %v", err)
	}
	if fcfg.Bind != "127.0.0.1" || !fcfg.Verbose || fcfg.Token.String() != "parsed:abc" {
		t.Fatalf("unexpected flatten cfg: %+v token=%q", fcfg, fcfg.Token.String())
	}
	if fcfg.File != "input.txt" || len(fcfg.Rest) != 2 || fcfg.Rest[0] != "extra1" || fcfg.Rest[1] != "extra2" {
		t.Fatalf("unexpected positional args: file=%q rest=%v", fcfg.File, fcfg.Rest)
	}

	var fcfg2 flattenConfig
	fp2, err := argweave.New(&fcfg2, app)
	if err != nil {
		t.Fatalf("New (negate): %v", err)
	}
	if _, err := fp2.Parse([]string{"-v", "--no-verbose", "input.txt"}); err != nil {
		t.Fatalf("Parse (negate): %v", err)
	}
	if fcfg2.Verbose {
		t.Fatalf("expected --no-verbose to force Verbose false, got %+v", fcfg2)
	}

	// 6. A missing required positional argument is a validation error.
	var fcfg3 flattenConfig
	fp3, err := argweave.New(&fcfg3, app)
	if err != nil {
		t.Fatalf("New (missing positional): %v", err)
	}
	if _, err := fp3.Parse(nil); err == nil {
		t.Fatal("expected error for missing required positional <FILE>, got nil")
	}

	// 7. --print-config prints the resolved config as JSON and returns
	// COMMAND_PRINT_CONFIG.
	var fcfg4 flattenConfig
	fp4, err := argweave.New(&fcfg4, app)
	if err != nil {
		t.Fatalf("New (print-config): %v", err)
	}
	if command, err := fp4.Parse([]string{"input.txt", "--print-config"}); command != argweave.COMMAND_PRINT_CONFIG || err != nil {
		t.Fatalf("expected CommandPrintConfig, got command=%v err=%v", command, err)
	}

	// 8. Providers: by default (Env, Flags, Default) an environment
	// variable takes precedence over a CLI flag for the same field.
	os.Setenv("TESTAPP_VALUE", "from-env")
	t.Cleanup(func() { os.Unsetenv("TESTAPP_VALUE") })

	var pcfg providerConfig
	pp, err := argweave.New(&pcfg, app)
	if err != nil {
		t.Fatalf("New (providers default): %v", err)
	}
	if _, err := pp.Parse([]string{"--value", "from-cli"}); err != nil {
		t.Fatalf("Parse (providers default): %v", err)
	}
	if pcfg.Value != "from-env" {
		t.Fatalf("expected env to win by default, got %q", pcfg.Value)
	}

	// 9. Reordering AppConfig.Providers changes precedence: flags before
	// env makes the CLI value win instead.
	flagsFirst := app
	flagsFirst.Providers = []argweave.ProviderKind{argweave.ProviderFlags, argweave.ProviderEnv, argweave.ProviderDefault}

	var pcfg2 providerConfig
	pp2, err := argweave.New(&pcfg2, flagsFirst)
	if err != nil {
		t.Fatalf("New (providers flags-first): %v", err)
	}
	if _, err := pp2.Parse([]string{"--value", "from-cli"}); err != nil {
		t.Fatalf("Parse (providers flags-first): %v", err)
	}
	if pcfg2.Value != "from-cli" {
		t.Fatalf("expected CLI to win with flags-first providers, got %q", pcfg2.Value)
	}

	// 10. Removing a provider disables that source entirely: dropping
	// ProviderDefault makes a required field with only a default value
	// fail to resolve once env/CLI are absent.
	os.Unsetenv("TESTAPP_VALUE")
	noDefault := app
	noDefault.Providers = []argweave.ProviderKind{argweave.ProviderEnv, argweave.ProviderFlags}

	var pcfg3 providerConfig
	pp3, err := argweave.New(&pcfg3, noDefault)
	if err != nil {
		t.Fatalf("New (providers no-default): %v", err)
	}
	if _, err := pp3.Parse(nil); err == nil {
		t.Fatal("expected missing required error with ProviderDefault removed, got nil")
	}

	// 11. An unknown provider kind is rejected at New().
	if _, err := argweave.New(&providerConfig{}, argweave.AppConfig{Providers: []argweave.ProviderKind{"bogus"}}); err == nil {
		t.Fatal("expected error for unknown provider kind, got nil")
	}

	// 12. The file provider (position 3 by default) loads values from a
	// JSON config file passed via --config.
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(jsonPath, []byte(`{"name":"from-json","tags":["x","y"]}`), 0o600); err != nil {
		t.Fatalf("writing json config: %v", err)
	}

	var fcfg5 fileConfig
	fp5, err := argweave.New(&fcfg5, app)
	if err != nil {
		t.Fatalf("New (file provider json): %v", err)
	}
	if _, err := fp5.Parse([]string{"--config", jsonPath}); err != nil {
		t.Fatalf("Parse (file provider json): %v", err)
	}
	if fcfg5.Name != "from-json" || len(fcfg5.Tags) != 2 || fcfg5.Tags[0] != "x" || fcfg5.Tags[1] != "y" {
		t.Fatalf("unexpected cfg from json config: %+v", fcfg5)
	}

	// 13. TOML config files are supported too, and CLI flags still outrank
	// the file provider per the default order (Env, Flags, File, Default).
	tomlPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(tomlPath, []byte("name = \"from-toml\"\n"), 0o600); err != nil {
		t.Fatalf("writing toml config: %v", err)
	}

	var fcfg6 fileConfig
	fp6, err := argweave.New(&fcfg6, app)
	if err != nil {
		t.Fatalf("New (file provider toml): %v", err)
	}
	if _, err := fp6.Parse([]string{"-c", tomlPath, "--name", "from-cli"}); err != nil {
		t.Fatalf("Parse (file provider toml): %v", err)
	}
	if fcfg6.Name != "from-cli" {
		t.Fatalf("expected CLI to outrank file provider, got %q", fcfg6.Name)
	}

	// 14. A missing --config file is a parse error.
	var fcfg7 fileConfig
	fp7, err := argweave.New(&fcfg7, app)
	if err != nil {
		t.Fatalf("New (file provider missing): %v", err)
	}
	if _, err := fp7.Parse([]string{"--config", filepath.Join(dir, "missing.json")}); err == nil {
		t.Fatal("expected error for missing --config file, got nil")
	}

	// 15. Disabling ProviderFile also disables the --config/-c flag itself.
	noFile := app
	noFile.Providers = []argweave.ProviderKind{argweave.ProviderEnv, argweave.ProviderFlags, argweave.ProviderDefault}
	var fcfg8 fileConfig
	fp8, err := argweave.New(&fcfg8, noFile)
	if err != nil {
		t.Fatalf("New (no file provider): %v", err)
	}
	if _, err := fp8.Parse([]string{"--config", jsonPath}); err == nil {
		t.Fatal("expected --config to be an unknown flag when ProviderFile is disabled")
	}

	// 16. Large integers in a JSON config file round-trip exactly (no
	// float64 precision loss), and fields are also matched by alias.
	precisionPath := filepath.Join(dir, "precision.json")
	if err := os.WriteFile(precisionPath, []byte(`{"big-id":9223372036854775000,"n":"from-alias"}`), 0o600); err != nil {
		t.Fatalf("writing precision config: %v", err)
	}

	var fcfg9 fileConfig2
	fp9, err := argweave.New(&fcfg9, app)
	if err != nil {
		t.Fatalf("New (file provider precision): %v", err)
	}
	if _, err := fp9.Parse([]string{"--config", precisionPath}); err != nil {
		t.Fatalf("Parse (file provider precision): %v", err)
	}
	if fcfg9.BigID != 9223372036854775000 {
		t.Fatalf("expected exact big-id round-trip, got %d", fcfg9.BigID)
	}
	if fcfg9.Name != "from-alias" {
		t.Fatalf("expected Name resolved via alias %q, got %q", "n", fcfg9.Name)
	}
	if fcfg9.Timeout != 5*time.Second {
		t.Fatalf("expected default Timeout, got %v", fcfg9.Timeout)
	}

	// 17. ConfigJSON uses config-file-compatible (kebab-case) keys and
	// renders time.Duration as a friendly string, not a raw int64.
	data, err := fp9.ConfigJSON()
	if err != nil {
		t.Fatalf("ConfigJSON: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshaling ConfigJSON output: %v", err)
	}
	if decoded["timeout"] != "5s" {
		t.Fatalf("expected ConfigJSON timeout %q, got %v", "5s", decoded["timeout"])
	}
	if _, ok := decoded["big-id"]; !ok {
		t.Fatalf("expected ConfigJSON to use kebab-case key %q, got keys %v", "big-id", decoded)
	}

	// 18. Sources reports which provider resolved each field.
	sources := fp9.Sources()
	if sources["big-id"] != argweave.ProviderFile {
		t.Fatalf("expected big-id resolved from %q, got %q", argweave.ProviderFile, sources["big-id"])
	}
	if sources["timeout"] != argweave.ProviderDefault {
		t.Fatalf("expected timeout resolved from %q, got %q", argweave.ProviderDefault, sources["timeout"])
	}

	// 19. GenerateCompletion renders bash/zsh/fish scripts listing flags,
	// and rejects unsupported shells.
	for _, shell := range []string{"bash", "zsh", "fish"} {
		script, err := fp9.GenerateCompletion(shell)
		if err != nil {
			t.Fatalf("GenerateCompletion(%q): %v", shell, err)
		}
		if !strings.Contains(script, "big-id") {
			t.Fatalf("GenerateCompletion(%q) missing flag, got:\n%s", shell, script)
		}
	}
	if script, err := fp9.GenerateCompletion("powershell"); err != nil || !strings.Contains(script, "Register-ArgumentCompleter") {
		t.Fatalf("expected PowerShell completion, script=%q err=%v", script, err)
	}

	// 20. AppConfig.ConfigSearchPaths lets the file provider look for the
	// auto-detected config file outside the working directory.
	searchDir := t.TempDir()
	base := strings.TrimSuffix(filepath.Base(os.Args[0]), filepath.Ext(os.Args[0]))
	if err := os.WriteFile(filepath.Join(searchDir, base+".json"), []byte(`{"name":"from-search-path"}`), 0o600); err != nil {
		t.Fatalf("writing auto-detected config: %v", err)
	}

	searchApp := app
	searchApp.ConfigSearchPaths = []string{searchDir}
	var fcfg10 fileConfig
	fp10, err := argweave.New(&fcfg10, searchApp)
	if err != nil {
		t.Fatalf("New (config search paths): %v", err)
	}
	if _, err := fp10.Parse(nil); err != nil {
		t.Fatalf("Parse (config search paths): %v", err)
	}
	if fcfg10.Name != "from-search-path" {
		t.Fatalf("expected auto-detected config from custom search path, got %q", fcfg10.Name)
	}

	// 21. A named (non-anonymous), untagged struct field - e.g. a
	// DatabaseConfig grouped inside the top-level config - is flattened
	// just like an embedded one, and works through CLI flags, env vars
	// (including AppConfig.EnvPrefix) and the file provider alike.
	var dcfg dbAppConfig
	dp, err := argweave.New(&dcfg, app)
	if err != nil {
		t.Fatalf("New (named nested struct): %v", err)
	}
	if _, err := dp.Parse([]string{"--db-host", "dbhost1", "--name", "svc"}); err != nil {
		t.Fatalf("Parse (named nested struct via flags): %v", err)
	}
	if dcfg.DB.Host != "dbhost1" || dcfg.DB.Port != 5432 || dcfg.Name != "svc" {
		t.Fatalf("unexpected nested struct cfg after flags: %+v", dcfg)
	}

	// 22. Same nested field, resolved from an environment variable, with
	// AppConfig.EnvPrefix applied uniformly (blank by default, no
	// difference; here explicitly configured).
	os.Setenv("MYAPP_DB_PORT", "6543")
	t.Cleanup(func() { os.Unsetenv("MYAPP_DB_PORT") })

	prefixedApp := app
	prefixedApp.EnvPrefix = "MYAPP_"
	var dcfg2 dbAppConfig
	dp2, err := argweave.New(&dcfg2, prefixedApp)
	if err != nil {
		t.Fatalf("New (nested struct + EnvPrefix): %v", err)
	}
	if _, err := dp2.Parse([]string{"--name", "svc"}); err != nil {
		t.Fatalf("Parse (nested struct + EnvPrefix): %v", err)
	}
	if dcfg2.DB.Port != 6543 {
		t.Fatalf("expected DB.Port from prefixed env var, got %d", dcfg2.DB.Port)
	}

	// 23. Same nested field, resolved from a config file via --config.
	dbConfigPath := filepath.Join(dir, "db.json")
	if err := os.WriteFile(dbConfigPath, []byte(`{"db-host":"from-file","name":"svc"}`), 0o600); err != nil {
		t.Fatalf("writing db config: %v", err)
	}
	var dcfg3 dbAppConfig
	dp3, err := argweave.New(&dcfg3, app)
	if err != nil {
		t.Fatalf("New (nested struct + file provider): %v", err)
	}
	if _, err := dp3.Parse([]string{"--config", dbConfigPath}); err != nil {
		t.Fatalf("Parse (nested struct + file provider): %v", err)
	}
	if dcfg3.DB.Host != "from-file" {
		t.Fatalf("expected DB.Host from config file, got %q", dcfg3.DB.Host)
	}
}

func TestValidationRelationshipsAndMetadata(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secretPath, []byte("from-file\n"), 0o600); err != nil {
		t.Fatalf("writing secret file: %v", err)
	}
	var cfg validationConfig
	parser, err := argweave.New(&cfg, argweave.AppConfig{Providers: []argweave.ProviderKind{argweave.ProviderFlags}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := parser.Parse([]string{"--tls", "--cert", "certificate", "--secret", secretPath}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Secret != "from-file" || cfg.ValidateCalls != 1 {
		t.Fatalf("unexpected validated config: %+v", cfg)
	}
	data, err := parser.ConfigJSON()
	if err != nil {
		t.Fatalf("ConfigJSON: %v", err)
	}
	if strings.Contains(string(data), "from-file") || !strings.Contains(string(data), "[REDACTED]") {
		t.Fatalf("secret was not redacted: %s", data)
	}
	help := parser.GenerateHelp()
	if !strings.Contains(help, "SECURITY:") || !strings.Contains(help, "--secret") {
		t.Fatalf("grouped help missing security section: %s", help)
	}

	var invalid validationConfig
	invalidParser, err := argweave.New(&invalid, argweave.AppConfig{Providers: []argweave.ProviderKind{argweave.ProviderFlags}})
	if err != nil {
		t.Fatalf("New invalid: %v", err)
	}
	if _, err := invalidParser.Parse([]string{"--cert", "certificate"}); err == nil {
		t.Fatal("expected cert requires tls validation error")
	}
	if _, err := invalidParser.Parse([]string{"--tls", "--cert", "certificate", "--insecure"}); err == nil {
		t.Fatal("expected insecure conflict validation error")
	}
}

func TestValidationMetadataFailsAtNewAndCallbackRuns(t *testing.T) {
	var invalid relationshipConfig
	if _, err := argweave.New(&invalid, argweave.AppConfig{}); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected invalid relationship error from New, got %v", err)
	}

	var cfg callbackConfig
	parser, err := argweave.New(&cfg, argweave.AppConfig{
		Providers: []argweave.ProviderKind{argweave.ProviderFlags},
		Validate: func(value interface{}) []error {
			candidate := value.(*callbackConfig)
			var validationErrors []error
			if candidate.Port < 1024 {
				validationErrors = append(validationErrors, fmt.Errorf("port must be >= 1024"))
			}
			if candidate.Port%2 != 0 {
				validationErrors = append(validationErrors, fmt.Errorf("port must be even"))
			}
			return validationErrors
		},
	})
	if err != nil {
		t.Fatalf("New callback: %v", err)
	}
	if _, err := parser.Parse([]string{"--port", "81"}); err == nil || !strings.Contains(err.Error(), "configuration validation failed") || !strings.Contains(err.Error(), "port must be >= 1024") || !strings.Contains(err.Error(), "port must be even") {
		t.Fatalf("expected callback validation error, got %v", err)
	}
	if _, err := parser.Parse([]string{"--port", "8080"}); err != nil {
		t.Fatalf("valid callback configuration failed: %v", err)
	}
}

func TestValidationIgnoresNilErrorsAndAggregatesRelationships(t *testing.T) {
	var cfg struct {
		TLS  bool   `arg:"long=tls,requires=certificate,requires=key"`
		Cert string `arg:"long=certificate"`
		Key  string `arg:"long=key"`
	}
	parser, err := argweave.New(&cfg, argweave.AppConfig{
		Providers: []argweave.ProviderKind{argweave.ProviderFlags},
		Validate:  func(interface{}) []error { return []error{nil} },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := parser.Parse([]string{"--tls"}); err == nil || !strings.Contains(err.Error(), "requires --certificate") || !strings.Contains(err.Error(), "requires --key") {
		t.Fatalf("expected aggregated relationship errors, got %v", err)
	}
}

func TestSourcesDetailedReportsProviderContext(t *testing.T) {
	var cfg struct {
		Port   int    `arg:"long=port,env=TEST_PORT"`
		Secret string `arg:"long=secret,secret"`
	}
	parser, err := argweave.New(&cfg, argweave.AppConfig{Providers: []argweave.ProviderKind{argweave.ProviderFlags}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := parser.Parse([]string{"--port", "8080", "--secret", "value"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	info := parser.SourcesDetailed()
	if info["port"].Provider != argweave.ProviderFlags || info["port"].Key != "--port" {
		t.Fatalf("unexpected port source: %+v", info["port"])
	}
	if !info["secret"].Redacted {
		t.Fatalf("expected secret source to be marked redacted: %+v", info["secret"])
	}
}

func TestParseErrorIncludesProvider(t *testing.T) {
	var cfg struct {
		Port int `arg:"long=port,default=not-a-number"`
	}
	parser, err := argweave.New(&cfg, argweave.AppConfig{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := parser.Parse(nil); err == nil || !strings.Contains(err.Error(), "from default") {
		t.Fatalf("expected provider in conversion error, got %v", err)
	}
}

func TestParsingDurationIsRecorded(t *testing.T) {
	var cfg struct {
		Name string `arg:"long=name"`
	}
	parser, err := argweave.New(&cfg, argweave.AppConfig{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if duration := parser.ParsingDuration(); duration != 0 {
		t.Fatalf("expected zero duration before parsing, got %s", duration)
	}
	if _, err := parser.Parse([]string{"--name", "example"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if duration := parser.ParsingDuration(); duration <= 0 {
		t.Fatalf("expected positive parsing duration, got %s", duration)
	}
	if _, err := parser.Parse([]string{"--help"}); err != nil {
		t.Fatalf("Parse help: %v", err)
	}
	if duration := parser.ParsingDuration(); duration < 0 {
		t.Fatalf("expected non-negative help parsing duration, got %s", duration)
	}
}

// providerConfig is used to exercise AppConfig.Providers reordering.
type providerConfig struct {
	Value string `arg:"long=value,env=TESTAPP_VALUE,default=fallback,required,help=Value"`
}

// fileConfig is used to exercise the JSON/TOML config file provider.
type fileConfig struct {
	Name string   `arg:"long=name,default=fallback,help=Name"`
	Tags []string `arg:"long=tags,help=Tags"`
}

// fileConfig2 exercises large-integer precision, alias-based config file
// lookup, ConfigJSON key parity, and Sources().
type fileConfig2 struct {
	BigID   int64         `arg:"long=big-id,default=0,help=Big ID"`
	Timeout time.Duration `arg:"long=timeout,default=5s,help=Timeout"`
	Name    string        `arg:"long=name,alias=n,default=fallback,help=Name"`
}

// databaseConfig is injected as a plain named (non-embedded) field below,
// to exercise flattening of nested "sub-object" config groups.
type databaseConfig struct {
	Host string `arg:"long=db-host,env=DB_HOST,default=localhost,help=Database host"`
	Port int    `arg:"long=db-port,env=DB_PORT,default=5432,help=Database port"`
}

// dbAppConfig groups a databaseConfig under a named field, unlike
// flattenConfig's anonymous embedding.
type dbAppConfig struct {
	DB   databaseConfig
	Name string `arg:"long=name,help=App name"`
}

type widthConfig struct {
	Small         uint8           `arg:"long=small"`
	Signed        int8            `arg:"long=signed"`
	Ratio         float32         `arg:"long=ratio"`
	Token         *customValue    `arg:"long=token"`
	Tokens        []customValue   `arg:"long=tokens"`
	PointerTokens []*customValue  `arg:"long=pointer-tokens"`
	Durations     []time.Duration `arg:"long=durations"`
}

type recursiveConfig struct {
	Next *recursiveConfig
}

type validationConfig struct {
	TLS           bool   `arg:"long=tls,group=Security"`
	Cert          string `arg:"long=cert,requires=tls,group=Security"`
	Insecure      bool   `arg:"long=insecure,conflicts=tls,group=Security"`
	Secret        string `arg:"long=secret,file,secret,group=Security"`
	ValidateCalls int
}

func (c *validationConfig) Validate() []error {
	c.ValidateCalls++
	var validationErrors []error
	if c.TLS && c.Insecure {
		validationErrors = append(validationErrors, fmt.Errorf("tls and insecure cannot both be enabled"))
	}
	if c.TLS && c.Cert == "" {
		validationErrors = append(validationErrors, fmt.Errorf("tls requires a certificate"))
	}
	return validationErrors
}

type relationshipConfig struct {
	TLS  bool   `arg:"long=tls,requires=missing-field"`
	Name string `arg:"long=name"`
}

type callbackConfig struct {
	Port int `arg:"long=port"`
}
