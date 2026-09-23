// Command full demonstrates the complete argweave configuration surface:
// nested structs, providers, validation, relationships, custom values,
// grouped help, secrets, file-backed values, slices, positional arguments,
// and explicit process exit management.
//
// Try it with:
//
//	go run ./examples/full --help
//	go run ./examples/full --tls --certificate server.crt input.txt
//	go run ./examples/full --insecure --mode batch input.txt
//	go run ./examples/full --print-config input.txt
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/SR-G/argweave"
)

//go:generate go run ../../cmd/weavedoc -type=Config -file=main.go -out=CONFIG.md -title=Full-Example-Configuration

// DatabaseConfig is flattened into Config because it has no arg tag of its own.
type DatabaseConfig struct {
	Host string `arg:"long=db-host,env=DB_HOST,default=localhost,group=Database,help=Database host"`
	Port uint16 `arg:"long=db-port,env=DB_PORT,default=5432,group=Database,help=Database port"`
}

// deploymentMode demonstrates a custom Value implementation.
type deploymentMode string

func (m *deploymentMode) String() string { return string(*m) }

func (m *deploymentMode) Set(value string) error {
	value = strings.ToLower(value)
	switch value {
	case "server", "batch", "worker":
		*m = deploymentMode(value)
		return nil
	default:
		return fmt.Errorf("mode must be server, batch, or worker")
	}
}

// endpoint demonstrates a custom type inside a slice.
type endpoint struct{ URL string }

func (e *endpoint) String() string { return e.URL }

func (e *endpoint) Set(value string) error {
	if !strings.HasPrefix(value, "https://") {
		return fmt.Errorf("endpoint must use https://")
	}
	e.URL = value
	return nil
}

// Config uses every major argweave feature in one realistic application
// configuration.
type Config struct {
	Database DatabaseConfig

	// Server options are grouped in help and can be supplied by env or flags.
	Port    uint16         `arg:"short=p,long=port,env=PORT,default=8080,group=Server,help=HTTP listen port"`
	Host    string         `arg:"long=host,env=HOST,default=127.0.0.1,alias=bind,group=Server,help=HTTP listen address"`
	Mode    deploymentMode `arg:"long=mode,env=MODE,default=server,group=Server,help=Deployment mode"`
	Verbose bool           `arg:"short,long,group=Server,help=Enable verbose logging"`

	// Security demonstrates requires, conflicts, secret redaction, and file input.
	TLS         bool   `arg:"long=tls,group=Security,help=Enable TLS"`
	Certificate string `arg:"long=certificate,requires=tls,group=Security,help=TLS certificate path"`
	Insecure    bool   `arg:"long=insecure,conflicts=tls,group=Security,help=Allow insecure transport"`
	APIKey      string `arg:"long=api-key,env=API_KEY,required,secret,group=Security,help=API key"`
	SecretFile  string `arg:"long=secret-file,env=SECRET_FILE,file,secret,group=Security,help=Read a secret from a file"`

	// Collections accept comma-separated and repeated values.
	Endpoints   []endpoint      `arg:"long=endpoint,group=Runtime,help=HTTPS service endpoint"`
	RetryDelays []time.Duration `arg:"long=retry-delay,default=1s,group=Runtime,help=Retry delays"`
	Tags        []string        `arg:"long=tags,env=TAGS,group=Runtime,help=Application tags"`

	// Hidden is parsed normally but omitted from help and generated docs.
	TraceID string `arg:"long=trace-id,hidden,help=Internal trace identifier"`

	InputFile string   `arg:"positional,required,value_name=file,help=Input file to process"`
	ExtraArgs []string `arg:"positional,help=Additional passthrough arguments"`
}

// Validate demonstrates whole-configuration validation after all providers
// and field relationships have been processed.
func (c *Config) Validate() []error {
	var validationErrors []error
	if c.Mode == "batch" && len(c.Endpoints) > 0 {
		validationErrors = append(validationErrors, fmt.Errorf("batch mode cannot configure endpoints"))
	}
	if c.TLS && c.Certificate == "" {
		validationErrors = append(validationErrors, fmt.Errorf("tls requires a certificate"))
	}
	return validationErrors
}

func main() {
	var cfg Config

	parser, err := argweave.New(&cfg, argweave.AppConfig{
		Name:        "full-example",
		Version:     "1.0.0",
		Description: "A complete argweave configuration example.",
		EnvPrefix:   "FULLAPP_",
	})
	if err != nil {
		// Parser construction errors indicate a programming/configuration error,
		// not user input.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(argweave.OS_EXIT_INTERNAL_ERROR)
	}

	command, err := parser.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(argweave.OS_EXIT_OPTIONS_IN_ERROR)
	}
	switch command {
	case argweave.COMMAND_HELP:
		fmt.Println(parser.GenerateHelp())
		os.Exit(argweave.OS_EXIT_OK)
	case argweave.COMMAND_VERSION:
		fmt.Println(parser.Version())
		os.Exit(argweave.OS_EXIT_OK)
	case argweave.COMMAND_PRINT_CONFIG:
		data, jsonErr := parser.ConfigJSON()
		if jsonErr != nil {
			fmt.Fprintln(os.Stderr, jsonErr)
			os.Exit(argweave.OS_EXIT_INTERNAL_ERROR)
		}
		fmt.Println(string(data))
		os.Exit(argweave.OS_EXIT_OK)
	}

	fmt.Printf("Starting %s on %s:%d in %s mode\n", cfg.InputFile, cfg.Host, cfg.Port, cfg.Mode)
	fmt.Printf("Loaded %d endpoint(s), %d retry delay(s), and %d tag(s)\n", len(cfg.Endpoints), len(cfg.RetryDelays), len(cfg.Tags))
	fmt.Printf("Parsing completed in %s\n", parser.ParsingDuration())
}
