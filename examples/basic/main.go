// Command basic demonstrates argweave: defining options via struct tags,
// parsing CLI flags and environment variables, and rendering --help.
//
// Run it with:
//
//	go run ./examples/basic --help
//	go run ./examples/basic --port 9090 --verbose input.txt
//	PORT=9090 go run ./examples/basic input.txt
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/SR-G/argweave"
)

//go:generate go run ../../cmd/weavedoc -type=Config -fields=long,short,env,default,required,description -file=main.go -out=CONFIG.md

// DatabaseConfig is a named "sub-object" nested inside Config below,
// demonstrating that nested config groups work through every provider.
type DatabaseConfig struct {
	Host string `arg:"long=db-host,env=DB_HOST,default=localhost,help=Database host"`
	Port int    `arg:"long=db-port,env=DB_PORT,default=5432,help=Database port"`
}

// Config holds every option accepted by this example program.
type Config struct {
	// DB groups database-related options; DatabaseConfig has no arg tag
	// of its own, so its fields are flattened into this Config.
	DB DatabaseConfig

	// Port the HTTP server listens on.
	Port int `arg:"short=p,long=port,env=PORT,default=8080,help=Port number to listen on"`

	// Host address the HTTP server binds to.
	Host string `arg:"long=host,env=HOST,default=localhost,alias=hostname,help=Host address to bind to"`

	// APIKey used to authenticate against the upstream service.
	APIKey string `arg:"long=api-key,env=API_KEY,required,help=API key for authentication"`

	// Verbose enables debug logging. --no-verbose forces it back off.
	Verbose bool `arg:"short,long,help=Enable verbose logging"`

	// Tags is a comma separated list of tags, or may be repeated: --tags a --tags b.
	Tags []string `arg:"long=tags,env=TAGS,help=List of tags\\, comma separated or repeated"`

	// Timeout demonstrates a custom type parsed via time.ParseDuration.
	Timeout time.Duration `arg:"long=timeout,default=30s,help=Request timeout"`

	// Secret is hidden from --help and generated docs, but still usable.
	Secret string `arg:"long=secret,help=Internal secret"`

	// InputFile is a required positional argument.
	InputFile string `arg:"positional,required,value_name=file,help=Input file to process"`

	// ExtraArgs collects any remaining positional arguments.
	ExtraArgs []string `arg:"positional,help=Additional passthrough arguments"`
}

func main() {
	var cfg Config

	parser, err := argweave.New(&cfg, argweave.AppConfig{
		Name:        "basic-example",
		Version:     "1.0.0",
		Description: "Example program demonstrating argweave.",
	})
	if err != nil {
		// These errors may especially be about wrong tags in the Config struct.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(argweave.OS_EXIT_INTERNAL_ERROR)
	}

	parser.ParseAndHandleExitIfNeeded(os.Args[1:])

	fmt.Printf("Parsed config : %+v\n", parser.DumpConfig())
}
