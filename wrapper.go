package argweave

import (
	"fmt"
	"io"
	"os"
)

// ExitCode associates a process exit code with a named purpose.
type ExitCode struct {
	Code    int
	Purpose string
}

// ExitCodesRepository stores the exit codes used by command handling.
type ExitCodesRepository struct {
	Codes map[string]ExitCode
}

// Retrieve returns the code registered for purpose, or -1 when it is absent.
func (ecr *ExitCodesRepository) Retrieve(purpose string) int {
	ec, ok := ecr.Codes[purpose]
	if !ok {
		return -1
	}
	return ec.Code
}

// Register associates code with purpose, replacing any existing value.
func (ecr *ExitCodesRepository) Register(code int, purpose string) {
	if ecr.Codes == nil {
		ecr.Codes = make(map[string]ExitCode)
	}
	ecr.Codes[purpose] = ExitCode{
		Code:    code,
		Purpose: purpose,
	}
}

// fillMissingDefaults registers the standard exit code for any of the
// three well-known purposes not already present in ecr, so a caller-supplied
// AppConfig.ExitCodes that only overrides some purposes never falls back to
// Retrieve's -1 "unknown purpose" sentinel for the others.
func (ecr *ExitCodesRepository) fillMissingDefaults() {
	defaults := map[string]int{
		EXIT_CODE_PURPOSE_NORMAL_EXIT:      OS_EXIT_OK,
		EXIT_CODE_PURPOSE_INTERNAL_ERROR:   OS_EXIT_INTERNAL_ERROR,
		EXIT_CODE_PURPOSE_OPTIONS_IN_ERROR: OS_EXIT_OPTIONS_IN_ERROR,
	}
	for purpose, code := range defaults {
		if _, ok := ecr.Codes[purpose]; !ok {
			ecr.Register(code, purpose)
		}
	}
}

// Handle parses args, writes built-in command output or errors to the supplied
// writers, and returns the configured exit code. It never terminates the
// process, making it suitable for libraries, tests, and embedded CLIs.
// This wrapper / handle method is a convenient method allowing to
// write shorter / simpler method (without implementing each time the full
// error handling logic in each main of each app using this library)
func (p *Parser) Handle(args []string, stdout, stderr io.Writer) int {
	command, err := p.Parse(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return p.exitCodes.Retrieve(EXIT_CODE_PURPOSE_OPTIONS_IN_ERROR)
	}
	if command != COMMAND_NONE {
		switch command {
		case COMMAND_HELP:
			fmt.Fprintln(stdout, p.GenerateHelp())
			return p.exitCodes.Retrieve(EXIT_CODE_PURPOSE_NORMAL_EXIT)
		case COMMAND_VERSION:
			fmt.Fprintln(stdout, p.Version())
			return p.exitCodes.Retrieve(EXIT_CODE_PURPOSE_NORMAL_EXIT)
		case COMMAND_PRINT_CONFIG:
			data, jsonErr := p.ConfigJSON()
			if jsonErr != nil {
				fmt.Fprintln(stderr, jsonErr)
				return p.exitCodes.Retrieve(EXIT_CODE_PURPOSE_INTERNAL_ERROR)
			}
			fmt.Fprintln(stdout, string(data))
			return p.exitCodes.Retrieve(EXIT_CODE_PURPOSE_NORMAL_EXIT)
		}
	}
	return p.exitCodes.Retrieve(EXIT_CODE_PURPOSE_NORMAL_EXIT)
}

// ParseAndHandleExitIfNeeded parses args, renders built-in command output or
// errors, and exits the process with the configured exit code when needed.
// It is the convenient process-level wrapper for command-line applications;
// use Parse or Handle when the caller must retain control of the process.
func (p *Parser) ParseAndHandleExitIfNeeded(args []string) {
	command, err := p.Parse(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(p.exitCodes.Retrieve(EXIT_CODE_PURPOSE_OPTIONS_IN_ERROR))
	}

	switch command {
	case COMMAND_HELP:
		fmt.Println(p.GenerateHelp())
		os.Exit(p.exitCodes.Retrieve(EXIT_CODE_PURPOSE_NORMAL_EXIT))
	case COMMAND_VERSION:
		fmt.Println(p.Version())
		os.Exit(p.exitCodes.Retrieve(EXIT_CODE_PURPOSE_NORMAL_EXIT))
	case COMMAND_PRINT_CONFIG:
		data, jsonErr := p.ConfigJSON()
		if jsonErr != nil {
			fmt.Fprintln(os.Stderr, jsonErr)
			os.Exit(p.exitCodes.Retrieve(EXIT_CODE_PURPOSE_INTERNAL_ERROR))
		}
		fmt.Println(string(data))
		os.Exit(p.exitCodes.Retrieve(EXIT_CODE_PURPOSE_NORMAL_EXIT))
	}
}
